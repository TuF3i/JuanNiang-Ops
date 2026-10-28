package config

import (
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ConfigManager 适配器配置管理: 各平台配置的建表 / 读取 / 更新 (gorm 持久化).
type ConfigManager struct {
	db *gorm.DB
}

func NewConfigManager(db *gorm.DB) *ConfigManager {
	return &ConfigManager{db: db}
}

// SetupConfig 初始化所有平台配置表, 并幂等写入各平台默认配置行.
func (r *ConfigManager) SetupConfig() error {
	if err := r.setupConfig(&OneBotConfig{}, defaultOneBotConfig()); err != nil {
		return err
	}
	if err := r.setupConfig(&FeishuConfig{}, defaultFeishuConfig()); err != nil {
		return err
	}
	return nil
}

// ---------- OneBot ----------

// UpdateConfig 保存 OneBot 配置 (cfg 必须携带主键, 先 GetConfig 再修改)
func (r *ConfigManager) UpdateConfig(cfg *OneBotConfig) error {
	return r.updateConfig(cfg)
}

// GetConfig 读取 OneBot 首条配置
func (r *ConfigManager) GetConfig() (*OneBotConfig, error) {
	return getConfig[OneBotConfig](r.db)
}

// ---------- 飞书 ----------

// UpdateFeishuConfig 保存飞书配置 (cfg 必须携带主键, 先 GetFeishuConfig 再修改)
func (r *ConfigManager) UpdateFeishuConfig(cfg *FeishuConfig) error {
	return r.updateConfig(cfg)
}

// GetFeishuConfig 读取飞书首条配置
func (r *ConfigManager) GetFeishuConfig() (*FeishuConfig, error) {
	return getConfig[FeishuConfig](r.db)
}

// ---------- 内部实现 ----------

// setupConfig 建表并幂等插入默认行 (主键冲突时跳过, 保证重复 Setup 安全).
// Clauses 必须挂在 Create 之前, 否则 SQL 已执行、子句不生效 (幂等插入失败).
func (r *ConfigManager) setupConfig(model, def any) error {
	if err := r.db.AutoMigrate(model); err != nil {
		return fmt.Errorf("setup config -> migrate %T: %w", model, err)
	}
	if err := r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(def).Error; err != nil {
		return fmt.Errorf("setup config -> seed %T: %w", model, err)
	}
	return nil
}

// updateConfig 保存配置 (cfg 必须携带主键, 否则 Save 会走 Insert 分支)
func (r *ConfigManager) updateConfig(cfg any) error {
	if err := r.db.Save(cfg).Error; err != nil {
		return fmt.Errorf("update config -> %w", err)
	}
	return nil
}

// getConfig 读取首条配置
func getConfig[T any](db *gorm.DB) (*T, error) {
	var data T
	err := db.First(&data).Error
	if err != nil {
		return nil, fmt.Errorf("get config -> %w", err)
	}
	return &data, nil
}
