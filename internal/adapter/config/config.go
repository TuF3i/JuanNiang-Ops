package config

import (
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ConfigManager struct {
	db *gorm.DB
}

func NewConfigManager(db *gorm.DB) *ConfigManager {
	return &ConfigManager{db: db}
}

func (r *ConfigManager) UpdateConfig(cfg *OneBotConfig) error {
	// cfg 必须携带主键 (先 GetConfig 再修改), 否则 Save 会走 Insert 分支
	if err := r.db.Save(cfg).Error; err != nil {
		return fmt.Errorf("update config -> %w", err)
	}
	return nil
}

func (r *ConfigManager) SetupConfig() error {
	if err := r.db.AutoMigrate(&OneBotConfig{}); err != nil {
		return fmt.Errorf("setup config -> %w", err)
	}

	defaultConfig := OneBotConfig{
		ID:     1,
		Addr:   defaultAddr,
		Token:  defaultToken,
		Admin:  []string{},
		Enable: defaultEnable,

		MessageBuffer: defaultMessageBuffer,
		ActionBuffer:  defaultActionBuffer,
	}

	// Clauses 必须挂在 Create 之前, 否则 SQL 已执行、子句不生效 (幂等插入失败)
	if err := r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&defaultConfig).Error; err != nil {
		return fmt.Errorf("setup config -> %w", err)
	}

	return nil
}

func (r *ConfigManager) GetConfig() (*OneBotConfig, error) {
	var data OneBotConfig

	err := r.db.First(&data).Error
	if err != nil {
		return nil, fmt.Errorf("get config -> %w", err)
	}

	return &data, nil
}
