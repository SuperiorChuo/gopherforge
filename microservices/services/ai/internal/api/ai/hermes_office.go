package ai

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/ai/internal/dao"
	"github.com/go-admin-kit/services/ai/internal/service/hermes"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

type HermesOfficeHandler struct {
	service *service.HermesOfficeService
}

func NewHermesOfficeHandler(s *service.HermesOfficeService) *HermesOfficeHandler {
	return &HermesOfficeHandler{service: s}
}

func (h *HermesOfficeHandler) ListFloors(c *gin.Context) {
	floors, err := h.service.ListFloors(c.Request.Context())
	if err != nil {
		writeOfficeError(c, err)
		return
	}
	response.Success(c, floors)
}

func (h *HermesOfficeHandler) CreateFloor(c *gin.Context) {
	var in service.FloorInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid floor payload")
		return
	}
	floor, err := h.service.CreateFloor(c.Request.Context(), in)
	if err != nil {
		writeOfficeError(c, err)
		return
	}
	response.Success(c, floor)
}

func (h *HermesOfficeHandler) UpdateFloor(c *gin.Context) {
	id, ok := officeIDParam(c, "id")
	if !ok {
		return
	}
	var in service.FloorInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid floor payload")
		return
	}
	if err := h.service.UpdateFloor(c.Request.Context(), id, in); err != nil {
		writeOfficeError(c, err)
		return
	}
	response.Success(c, nil)
}

func (h *HermesOfficeHandler) DeleteFloor(c *gin.Context) {
	id, ok := officeIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteFloor(c.Request.Context(), id); err != nil {
		writeOfficeError(c, err)
		return
	}
	response.Success(c, nil)
}

func (h *HermesOfficeHandler) ListSeats(c *gin.Context) {
	floorID, ok := officeIDParam(c, "id")
	if !ok {
		return
	}
	seats, err := h.service.ListSeats(c.Request.Context(), floorID)
	if err != nil {
		writeOfficeError(c, err)
		return
	}
	response.Success(c, seats)
}

func (h *HermesOfficeHandler) ReplaceSeats(c *gin.Context) {
	floorID, ok := officeIDParam(c, "id")
	if !ok {
		return
	}
	var body struct {
		Seats []service.SeatInput `json:"seats"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid seats payload")
		return
	}
	if err := h.service.ReplaceSeats(c.Request.Context(), floorID, body.Seats); err != nil {
		writeOfficeError(c, err)
		return
	}
	response.Success(c, nil)
}

func (h *HermesOfficeHandler) DeleteSeat(c *gin.Context) {
	seatID, ok := officeIDParam(c, "seat_id")
	if !ok {
		return
	}
	if err := h.service.DeleteSeat(c.Request.Context(), seatID); err != nil {
		writeOfficeError(c, err)
		return
	}
	response.Success(c, nil)
}

// HermesUsageHandler 暴露模型代理的用量统计（只有元数据，无请求正文）。
type HermesUsageHandler struct {
	service *service.HermesUsageService
}

func NewHermesUsageHandler(s *service.HermesUsageService) *HermesUsageHandler {
	return &HermesUsageHandler{service: s}
}

func (h *HermesUsageHandler) Overview(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))
	topN, _ := strconv.Atoi(c.DefaultQuery("top", "10"))
	result, err := h.service.Overview(c.Request.Context(), days, topN)
	if err != nil {
		response.InternalServerError(c, "load Hermes usage overview failed")
		return
	}
	response.Success(c, result)
}

func (h *HermesUsageHandler) DeviceUsage(c *gin.Context) {
	deviceID := c.Param("device_id")
	if deviceID == "" {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "device id is required")
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))
	result, err := h.service.DeviceUsage(c.Request.Context(), deviceID, days)
	if err != nil {
		response.InternalServerError(c, "load Hermes device usage failed")
		return
	}
	response.Success(c, result)
}

func officeIDParam(c *gin.Context, name string) (uint64, bool) {
	raw := c.Param(name)
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_INVALID_REQUEST"), "invalid id")
		return 0, false
	}
	return id, true
}

func writeOfficeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, dao.ErrOfficeFloorNotFound):
		response.NotFoundWithCode(c, response.ErrorCode("HERMES_OFFICE_FLOOR_NOT_FOUND"), "floor not found")
	case errors.Is(err, service.ErrOfficeSeatOutOfBounds):
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_OFFICE_SEAT_OUT_OF_BOUNDS"), "seat outside floor bounds")
	case errors.Is(err, service.ErrOfficeDuplicateUser):
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_OFFICE_DUPLICATE_USER"), "one user can only occupy one seat")
	case errors.Is(err, service.ErrOfficeInvalidFloorSize):
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_OFFICE_INVALID_FLOOR"), "invalid floor size")
	case errors.Is(err, service.ErrOfficeTooManySeats):
		response.BadRequestWithCode(c, response.ErrorCode("HERMES_OFFICE_TOO_MANY_SEATS"), "too many seats")
	default:
		response.InternalServerError(c, "hermes office request failed")
	}
}

// MyUsage 员工个人用量（M6）。身份来自会话，只统计本人调用。
func (h *HermesUsageHandler) MyUsage(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))
	result, err := h.service.MyUsage(c.Request.Context(), userID, days)
	if err != nil {
		response.InternalServerError(c, "load my Hermes usage failed")
		return
	}
	response.Success(c, result)
}
