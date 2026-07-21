package system

import (
	"context"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	systemsvc "github.com/go-admin-kit/services/identity/internal/service/system"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

// PostAPI handles job position endpoints.
type PostAPI struct {
	postService systemsvc.PostService
}

// NewPostAPI creates a PostAPI instance.
func NewPostAPI() *PostAPI {
	return &PostAPI{
		postService: systemsvc.PostService{},
	}
}

// NewPostAPIWithService creates a PostAPI instance from an injected service.
func NewPostAPIWithService(postService systemsvc.PostService) *PostAPI {
	return &PostAPI{postService: postService}
}

// GetPostList returns paginated posts.
func (a *PostAPI) GetPostList(c *gin.Context) {
	var req systemsvc.PostListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "invalid query parameters")
		return
	}

	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}

	// Parse status from query separately because Gin does not bind *int8 reliably.
	if statusStr := c.Query("status"); statusStr != "" {
		status, err := strconv.ParseInt(statusStr, 10, 8)
		if err == nil {
			statusInt8 := int8(status)
			req.Status = &statusInt8
		}
	}

	posts, total, err := a.postService.GetListContext(c.Request.Context(), req)
	if err != nil {
		internalServerError(c, "failed to get post list", err)
		return
	}

	response.PageSuccess(c, posts, total, req.Page, req.PageSize)
}

// GetAllPosts returns all posts, optionally filtered by status.
func (a *PostAPI) GetAllPosts(c *gin.Context) {
	var status *int8
	if statusStr := c.Query("status"); statusStr != "" {
		statusVal, err := strconv.ParseInt(statusStr, 10, 8)
		if err == nil {
			statusInt8 := int8(statusVal)
			status = &statusInt8
		}
	}

	posts, err := a.postService.GetAllContext(c.Request.Context(), status)
	if err != nil {
		internalServerError(c, "failed to get posts", err)
		return
	}

	response.Success(c, posts)
}

// GetPost returns a post by id.
func (a *PostAPI) GetPost(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid post id")
		return
	}

	post, err := a.postService.GetByIDContext(c.Request.Context(), uint(id))
	if err != nil {
		writeSystemPostServiceError(c, "failed to get post", err)
		return
	}

	response.Success(c, post)
}

// CreatePost creates a post.
func (a *PostAPI) CreatePost(c *gin.Context) {
	var req systemsvc.CreatePostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	post, err := a.postService.CreateContext(c.Request.Context(), req)
	if err != nil {
		writeSystemPostServiceError(c, "failed to create post", err)
		return
	}

	response.SuccessWithMessage(c, "post created", post)
}

// UpdatePost updates a post.
func (a *PostAPI) UpdatePost(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid post id")
		return
	}

	var req systemsvc.UpdatePostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	post, err := a.postService.UpdateContext(c.Request.Context(), uint(id), req)
	if err != nil {
		writeSystemPostServiceError(c, "failed to update post", err)
		return
	}

	response.SuccessWithMessage(c, "post updated", post)
}

// DeletePost deletes a post.
func (a *PostAPI) DeletePost(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid post id")
		return
	}

	if err := a.postService.DeleteContext(c.Request.Context(), uint(id)); err != nil {
		writeSystemPostServiceError(c, "failed to delete post", err)
		return
	}

	response.SuccessWithMessage(c, "post deleted", nil)
}

func writeSystemPostServiceError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, systemsvc.ErrPostCodeAlreadyExists):
		response.BadRequestWithCode(c, response.ErrorCodePostCodeAlreadyExists, systemsvc.ErrPostCodeAlreadyExists.Error())
	case errors.Is(err, systemsvc.ErrPostHasUsers):
		response.BadRequestWithCode(c, response.ErrorCodePostHasUsers, systemsvc.ErrPostHasUsers.Error())
	case errors.Is(err, systemsvc.ErrPostNotFound):
		response.NotFoundWithCode(c, response.ErrorCodePostNotFound, systemsvc.ErrPostNotFound.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		internalServerError(c, operation, err)
	default:
		internalServerError(c, operation, err)
	}
}
