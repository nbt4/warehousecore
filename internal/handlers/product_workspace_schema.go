package handlers

import (
	"context"
	_ "embed"
	"warehousecore/internal/repository"
	"warehousecore/internal/services"
)

//go:embed product_workspace.sql
var productWorkspaceSQL string

func EnsureProductWorkspaceSchema() error {
	db := repository.GetSQLDB()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(productWorkspaceSQL + services.MaterialSnapshotSchema); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	websiteRevalidator.Start(context.Background(), db)
	go resumePictureDeletions()
	return nil
}
