package planner

import (
	"github.com/joaoteixeira10/orbit/api/internal/models"

	"gorm.io/gorm"
)

// Every task list has a folder of its own (a project: the list's files,
// pages and sheets in one place), made with it. Someone can unjoin it later;
// then it stays so.

// ListFolder joins a list to a folder when none is joined to it: a folder at
// the top with the list's name and no list of its own, else a new one named
// and coloured as the list.
func ListFolder(tx *gorm.DB, l models.TaskList, by string) error {
	var n int64
	tx.Model(&models.Folder{}).Where("workspace_id = ? AND list_id = ?", l.WorkspaceID, l.ID).Count(&n)
	if n > 0 {
		return nil
	}
	var same models.Folder
	if tx.Where("workspace_id = ? AND parent_id = '' AND list_id = '' AND name = ?", l.WorkspaceID, l.Name).Limit(1).Find(&same).RowsAffected == 1 {
		return tx.Model(&models.Folder{}).Where("id = ?", same.ID).Update("list_id", l.ID).Error
	}
	return tx.Create(&models.Folder{WorkspaceID: l.WorkspaceID, Name: l.Name, Color: l.Color, ListID: l.ID, CreatedBy: by}).Error
}

// ListFolders gives every list of a workspace without a folder its own.
func ListFolders(tx *gorm.DB, wsID, by string) error {
	var lists []models.TaskList
	tx.Where("workspace_id = ?", wsID).Order("rank").Find(&lists)
	for _, l := range lists {
		if err := ListFolder(tx, l, by); err != nil {
			return err
		}
	}
	return nil
}

// RenameListFolder follows a list's new name (and colour) in its folder,
// while the folder still had the old one (someone may have named it apart).
func RenameListFolder(tx *gorm.DB, before, after models.TaskList) error {
	var f models.Folder
	if tx.Where("workspace_id = ? AND list_id = ?", before.WorkspaceID, before.ID).Limit(1).Find(&f).RowsAffected == 0 {
		return nil
	}
	up := map[string]any{}
	if f.Name == before.Name && after.Name != before.Name {
		up["name"] = after.Name
	}
	if f.Color == before.Color && after.Color != before.Color {
		up["color"] = after.Color
	}
	if len(up) == 0 {
		return nil
	}
	return tx.Model(&models.Folder{}).Where("id = ?", f.ID).Updates(up).Error
}

// FilesWith are the modules whose things live in folders: a workspace with
// any of them has Files on by default.
var FilesWith = []string{ModTasks, ModPages, ModSheets, ModDiagrams, ModBoards}
