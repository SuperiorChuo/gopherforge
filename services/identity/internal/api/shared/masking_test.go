package shared

import "testing"

func TestShouldMaskOwnProfile(t *testing.T) {
	targetUserID := uint(7)
	if ShouldMask(7, &targetUserID, nil) {
		t.Fatal("own profile should not be masked")
	}
}

func TestShouldMaskSuperAdmin(t *testing.T) {
	if ShouldMask(7, nil, []string{"operator", "super_admin"}) {
		t.Fatal("super_admin should not be masked")
	}
}

func TestShouldMaskOtherUserForNonSuperAdmin(t *testing.T) {
	targetUserID := uint(8)
	if !ShouldMask(7, &targetUserID, []string{"operator"}) {
		t.Fatal("other user should be masked for non-super-admin")
	}
}
