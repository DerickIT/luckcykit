package database

import (
	"gorm.io/gorm"
	"luckytv/backend/internal/model"
)

func migrateAssistantSkillContexts(tx *gorm.DB) error {
	return tx.AutoMigrate(&model.AssistantSkillContext{}, &model.AssistantSkillContextMutation{})
}
