package dao

import (
	"context"
	"fmt"

	"gorm.io/gorm/clause"

	"JuanNiang-Ops/internal/adapter/modules"
)

// SetupConfig 初始化所有平台配置表, 并幂等写入各平台默认配置行.
func (r *Dao) SetupConfig(ctx context.Context) error {
	// Clauses 必须挂在 Create 之前, 否则 SQL 已执行、子句不生效 (幂等插入失败)
	if err := r.setup(&modules.OneBotConfig{}, modules.DefaultOneBotConfig()); err != nil {
		return err
	}
	if err := r.setup(&modules.FeishuConfig{}, modules.DefaultFeishuConfig()); err != nil {
		return err
	}
	return nil
}

// setup 单个配置模型: AutoMigrate + 幂等种子行
func (r *Dao) setup(model, def any) error {
	if err := r.db.AutoMigrate(model); err != nil {
		return fmt.Errorf("adapter -> migrate %T: %w", model, err)
	}
	if err := r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(def).Error; err != nil {
		return fmt.Errorf("adapter -> seed %T: %w", model, err)
	}
	return nil
}

// GetOneBotConfig 读取 OneBot 首条配置
func (r *Dao) GetOneBotConfig(ctx context.Context) (*modules.OneBotConfig, error) {
	var data modules.OneBotConfig
	if err := r.db.WithContext(ctx).First(&data).Error; err != nil {
		return nil, fmt.Errorf("adapter -> get onebot config: %w", err)
	}
	return &data, nil
}

// UpdateOneBotConfig 保存 OneBot 配置 (cfg 必须携带主键, 先 Get 再改, 否则 Save 走 Insert 分支)
func (r *Dao) UpdateOneBotConfig(ctx context.Context, cfg *modules.OneBotConfig) error {
	if err := r.db.WithContext(ctx).Save(cfg).Error; err != nil {
		return fmt.Errorf("adapter -> update onebot config: %w", err)
	}
	return nil
}

// GetFeishuConfig 读取飞书首条配置
func (r *Dao) GetFeishuConfig(ctx context.Context) (*modules.FeishuConfig, error) {
	var data modules.FeishuConfig
	if err := r.db.WithContext(ctx).First(&data).Error; err != nil {
		return nil, fmt.Errorf("adapter -> get feishu config: %w", err)
	}
	return &data, nil
}

// UpdateFeishuConfig 保存飞书配置 (cfg 必须携带主键, 先 Get 再改)
func (r *Dao) UpdateFeishuConfig(ctx context.Context, cfg *modules.FeishuConfig) error {
	if err := r.db.WithContext(ctx).Save(cfg).Error; err != nil {
		return fmt.Errorf("adapter -> update feishu config: %w", err)
	}
	return nil
}
