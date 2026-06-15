package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Role struct {
	ID           int             `json:"id" gorm:"column:roleid;primaryKey"`
	Name         string          `json:"name" gorm:"column:name;not null"`
	IsSystemRole bool            `json:"is_system_role" gorm:"column:is_system_role"`
	Permissions  json.RawMessage `json:"permissions" gorm:"column:permissions;type:jsonb"`
}

func (Role) TableName() string {
	return "roles"
}

func main() {
	// FIXED: removed hardcoded password — this debug tool now requires DB_PASS env var
	dsn := fmt.Sprintf("host=localhost user=rentalcore password=%s dbname=rentalcore port=5432 sslmode=disable", os.Getenv("DB_PASS"))
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	var roles []Role
	if err := db.Find(&roles).Error; err != nil {
		log.Fatalf("failed to query roles: %v", err)
	}

	fmt.Printf("Found %d roles\n", len(roles))
	for _, r := range roles {
		fmt.Printf("Role: %s (ID: %d), System: %v, Perms: %s\n", r.Name, r.ID, r.IsSystemRole, string(r.Permissions))
	}

	var count int64
	db.Table("user_roles").Count(&count)
	fmt.Printf("UserRoles count: %d\n", count)

	var userRoles []Role
	userID := 1
	err = db.Table("roles").
		Joins("JOIN user_roles ON user_roles.roleid = roles.roleid").
		Where("user_roles.userid = ?", userID).
		Find(&userRoles).Error
	if err != nil {
		log.Fatalf("failed to query user roles: %v", err)
	}
	fmt.Printf("User %d has %d roles\n", userID, len(userRoles))
	for _, r := range userRoles {
		fmt.Printf(" - %s\n", r.Name)
	}
}
