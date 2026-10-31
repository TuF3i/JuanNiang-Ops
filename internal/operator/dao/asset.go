// 资产持久化: 建表与 CRUD.
// 与 adapter 的配置表不同: 资产是用户数据, 仅建表无幂等种子行.
package dao

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"JuanNiang-Ops/internal/operator/modules"
)

// Setup 建表 (幂等): 资产 + 媒介实体 + 通道实体 + 两张绑定表.
// 资产/媒介/通道是用户数据, 无幂等种子行 (与 adapter 配置表不同).
// 注: 旧版 operator_assets 的 media/channels 列由本模型移除, AutoMigrate 不删列, 残留无害.
func (r *Dao) Setup(ctx context.Context) error {
	if err := r.db.WithContext(ctx).AutoMigrate(
		&modules.Asset{},
		&modules.MediaSource{},
		&modules.ChannelSource{},
		&modules.AssetMediaBinding{},
		&modules.AssetChannelBinding{},
	); err != nil {
		return fmt.Errorf("operator -> migrate operator tables: %w", err)
	}
	return nil
}

// CreateAsset 新建资产 (ID 须由 service 预先填充; Name 唯一冲突报错)
func (r *Dao) CreateAsset(ctx context.Context, asset *modules.Asset) error {
	if err := r.db.WithContext(ctx).Create(asset).Error; err != nil {
		return fmt.Errorf("operator -> create asset %s: %w", asset.Name, err)
	}
	return nil
}

// GetAsset 按 ID 查询资产 (软删后不可见; 未找到返回 gorm.ErrRecordNotFound)
func (r *Dao) GetAsset(ctx context.Context, id string) (*modules.Asset, error) {
	var asset modules.Asset
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&asset).Error; err != nil {
		return nil, fmt.Errorf("operator -> get asset %s: %w", id, err)
	}
	return &asset, nil
}

// ListAssets 全量资产列表 (资产数量级小, tags 过滤由 service 内存完成)
func (r *Dao) ListAssets(ctx context.Context) ([]modules.Asset, error) {
	var assets []modules.Asset
	if err := r.db.WithContext(ctx).Order("created_at").Find(&assets).Error; err != nil {
		return nil, fmt.Errorf("operator -> list assets: %w", err)
	}
	return assets, nil
}

// ListEnabledAssets 已启用资产列表 (LoadEnabled 用)
func (r *Dao) ListEnabledAssets(ctx context.Context) ([]modules.Asset, error) {
	var assets []modules.Asset
	if err := r.db.WithContext(ctx).Where("enabled = ?", true).Find(&assets).Error; err != nil {
		return nil, fmt.Errorf("operator -> list enabled assets: %w", err)
	}
	return assets, nil
}

// UpdateAsset 全量更新 (asset 须携带主键, 先 Get 再改, 否则 Save 走 Insert 分支)
func (r *Dao) UpdateAsset(ctx context.Context, asset *modules.Asset) error {
	if err := r.db.WithContext(ctx).Save(asset).Error; err != nil {
		return fmt.Errorf("operator -> update asset %s: %w", asset.ID, err)
	}
	return nil
}

// DeleteAsset 软删资产 (绑定行由 service 先行清理, 此处兜底再清一次防孤儿)
func (r *Dao) DeleteAsset(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Delete(&modules.Asset{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("operator -> delete asset %s: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("operator -> delete asset %s: %w", id, gorm.ErrRecordNotFound)
	}
	if err := r.DeleteAssetMediaBindings(ctx, id); err != nil {
		return err
	}
	return r.DeleteAssetChannelBindings(ctx, id)
}

// SetEnabled 更新启用开关 (RowsAffected==0 表示资产不存在)
func (r *Dao) SetEnabled(ctx context.Context, id string, enabled bool) error {
	result := r.db.WithContext(ctx).Model(&modules.Asset{}).
		Where("id = ?", id).
		Update("enabled", enabled)
	if result.Error != nil {
		return fmt.Errorf("operator -> set asset %s enabled=%v: %w", id, enabled, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("operator -> set asset %s enabled: %w", id, gorm.ErrRecordNotFound)
	}
	return nil
}
