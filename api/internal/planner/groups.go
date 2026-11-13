package planner

import (
	"errors"

	"github.com/joaoteixeira10/orbit/api/internal/models"

	"gorm.io/gorm"
)

// Groups in workspaces: a group added to a workspace with a role makes its
// people members there. A membership row stays one per person — every query
// that asks "who's in it" keeps working — with Role the highest of the role
// given to them (DirectRole) and their groups' roles. SyncGroupAccess keeps
// the rows so after any change to groups, their people, or direct roles.

// ErrNoOwner: the change would leave a workspace without an owner.
var ErrNoOwner = errors.New("a workspace needs at least one owner; make someone else an owner first")

// RoleRank orders workspace roles (higher may do more; "" none).
func RoleRank(r string) int {
	switch r {
	case models.WSOwner:
		return 3
	case models.WSEditor:
		return 2
	case models.WSViewer:
		return 1
	}
	return 0
}

// HigherRole is the higher of two workspace roles.
func HigherRole(a, b string) string { return higher(a, b) }

func higher(a, b string) string {
	if RoleRank(b) > RoleRank(a) {
		return b
	}
	return a
}

// SyncGroupAccess brings a workspace's membership rows in line with its
// direct roles and its groups: adds who a group brings in, sets each one's
// role, removes who no longer has either. It returns the people whose access
// changed (added, removed or another role), and ErrNoOwner — changing
// nothing, in a transaction — when no owner would be left.
func SyncGroupAccess(tx *gorm.DB, wsID string) ([]string, error) {
	var viaGroups []struct {
		UserID string
		Role   string
	}
	if err := tx.Table("workspace_groups").
		Select("group_members.user_id, workspace_groups.role").
		Joins("JOIN group_members ON group_members.group_id = workspace_groups.group_id").
		Joins("JOIN users ON users.id = group_members.user_id AND users.disabled = ?", false).
		Where("workspace_groups.workspace_id = ?", wsID).Scan(&viaGroups).Error; err != nil {
		return nil, err
	}
	want := map[string]string{}
	for _, g := range viaGroups {
		want[g.UserID] = higher(want[g.UserID], g.Role)
	}
	var rows []models.WorkspaceMember
	if err := tx.Where("workspace_id = ?", wsID).Find(&rows).Error; err != nil {
		return nil, err
	}
	var changed []string
	have := map[string]bool{}
	owners := 0
	for _, m := range rows {
		have[m.UserID] = true
		role := higher(m.DirectRole, want[m.UserID])
		switch {
		case role == "":
			if err := tx.Delete(&m).Error; err != nil {
				return nil, err
			}
			changed = append(changed, m.UserID)
			continue
		case role != m.Role:
			if err := tx.Model(&m).Update("role", role).Error; err != nil {
				return nil, err
			}
			changed = append(changed, m.UserID)
		}
		if role == models.WSOwner {
			owners++
		}
	}
	for uid, role := range want {
		if have[uid] {
			continue
		}
		var last struct{ Max *float64 }
		tx.Model(&models.WorkspaceMember{}).Select("MAX(position) AS max").Where("user_id = ?", uid).Scan(&last)
		if err := tx.Create(&models.WorkspaceMember{WorkspaceID: wsID, UserID: uid, Role: role, Position: Rank(last.Max, nil)}).Error; err != nil {
			return nil, err
		}
		changed = append(changed, uid)
		if role == models.WSOwner {
			owners++
		}
	}
	if owners == 0 && len(rows) > 0 {
		return nil, ErrNoOwner
	}
	return changed, nil
}

// GroupWorkspaces: the workspaces a group is in.
func GroupWorkspaces(tx *gorm.DB, groupID string) []string {
	var ids []string
	tx.Model(&models.WorkspaceGroup{}).Where("group_id = ?", groupID).Pluck("workspace_id", &ids)
	return ids
}

// UserGroupWorkspaces: the workspaces someone is in through any group of theirs.
func UserGroupWorkspaces(tx *gorm.DB, userID string) []string {
	var ids []string
	tx.Model(&models.WorkspaceGroup{}).Distinct("workspace_id").
		Where("group_id IN (?)", tx.Model(&models.GroupMember{}).Select("group_id").Where("user_id = ?", userID)).
		Pluck("workspace_id", &ids)
	return ids
}
