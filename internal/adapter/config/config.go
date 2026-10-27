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
	err := r.db.First(&OneBotConfig{}).Save(cfg).Error
	return fmt.Errorf("update config -> %v", err.Error())
}

func (r *ConfigManager) SetupConfig() error {
	err := r.db.AutoMigrate(&OneBotConfig{})
	if err != nil {
		return fmt.Errorf("setup config -> %v", err.Error())
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

	err = r.db.Create(&defaultConfig).Clauses(clause.OnConflict{DoNothing: true}).Error
	if err != nil {
		return fmt.Errorf("setup config -> %v", err.Error())
	}

	return nil
}

func (r *ConfigManager) GetConfig() (*OneBotConfig, error) {
	var data OneBotConfig

	err := r.db.First(&data).Error
	if err != nil {
		return nil, fmt.Errorf("get config -> %v", err.Error())
	}

	return &data, nil
}
