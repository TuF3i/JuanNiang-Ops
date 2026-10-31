// 数据通道实体持久化: CRUD 与资产绑定 (与 media.go 同构).
package dao

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"JuanNiang-Ops/internal/operator/modules"
)

// CreateChannelSource 新建通道实体
func (r *Dao) CreateChannelSource(ctx context.Context, src *modules.ChannelSource) error {
	if err := r.db.WithContext(ctx).Create(src).Error; err != nil {
		return fmt.Errorf("operator -> create channel source %s: %w", src.Name, err)
	}
	return nil
}

// GetChannelSource 按 ID 查询通道实体 (未找到返回 gorm.ErrRecordNotFound)
func (r *Dao) GetChannelSource(ctx context.Context, id string) (*modules.ChannelSource, error) {
	var src modules.ChannelSource
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&src).Error; err != nil {
		return nil, fmt.Errorf("operator -> get channel source %s: %w", id, err)
	}
	return &src, nil
}

// ListChannelSources 通道实体列表 (kind 非空时按类型过滤)
func (r *Dao) ListChannelSources(ctx context.Context, kind modules.ChannelKind) ([]modules.ChannelSource, error) {
	query := r.db.WithContext(ctx).Model(&modules.ChannelSource{})
	if kind != "" {
		query = query.Where("kind = ?", kind)
	}
	var sources []modules.ChannelSource
	if err := query.Order("created_at").Find(&sources).Error; err != nil {
		return nil, fmt.Errorf("operator -> list channel sources: %w", err)
	}
	return sources, nil
}

// UpdateChannelSource 全量更新 (src 须携带主键)
func (r *Dao) UpdateChannelSource(ctx context.Context, src *modules.ChannelSource) error {
	if err := r.db.WithContext(ctx).Save(src).Error; err != nil {
		return fmt.Errorf("operator -> update channel source %s: %w", src.ID, err)
	}
	return nil
}

// DeleteChannelSource 软删通道实体 (绑定关系由 service 在删除前校验并清理)
func (r *Dao) DeleteChannelSource(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Delete(&modules.ChannelSource{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("operator -> delete channel source %s: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("operator -> delete channel source %s: %w", id, gorm.ErrRecordNotFound)
	}
	return nil
}

// ListChannelSourcesByAsset 资产绑定的全部通道实体
func (r *Dao) ListChannelSourcesByAsset(ctx context.Context, assetID string) ([]modules.ChannelSource, error) {
	var bindings []modules.AssetChannelBinding
	if err := r.db.WithContext(ctx).Where("asset_id = ?", assetID).Find(&bindings).Error; err != nil {
		return nil, fmt.Errorf("operator -> list channel bindings for asset %s: %w", assetID, err)
	}
	if len(bindings) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(bindings))
	for _, b := range bindings {
		ids = append(ids, b.ChannelSourceID)
	}
	var sources []modules.ChannelSource
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&sources).Error; err != nil {
		return nil, fmt.Errorf("operator -> load channel sources for asset %s: %w", assetID, err)
	}
	return sources, nil
}

// ListAssetIDsByChannelSource 反向查询: 绑定了该通道的资产 ID 列表
func (r *Dao) ListAssetIDsByChannelSource(ctx context.Context, channelSourceID string) ([]string, error) {
	var bindings []modules.AssetChannelBinding
	if err := r.db.WithContext(ctx).Where("channel_source_id = ?", channelSourceID).Find(&bindings).Error; err != nil {
		return nil, fmt.Errorf("operator -> list bindings for channel source %s: %w", channelSourceID, err)
	}
	ids := make([]string, 0, len(bindings))
	for _, b := range bindings {
		ids = append(ids, b.AssetID)
	}
	return ids, nil
}

// BindAssetChannel 绑定通道到资产 (OnConflict DoNothing 幂等)
func (r *Dao) BindAssetChannel(ctx context.Context, assetID, channelSourceID string) error {
	binding := modules.AssetChannelBinding{AssetID: assetID, ChannelSourceID: channelSourceID}
	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&binding).Error; err != nil {
		return fmt.Errorf("operator -> bind channel %s to asset %s: %w", channelSourceID, assetID, err)
	}
	return nil
}

// UnbindAssetChannel 解除资产与通道的绑定 (幂等)
func (r *Dao) UnbindAssetChannel(ctx context.Context, assetID, channelSourceID string) error {
	if err := r.db.WithContext(ctx).
		Where("asset_id = ? AND channel_source_id = ?", assetID, channelSourceID).
		Delete(&modules.AssetChannelBinding{}).Error; err != nil {
		return fmt.Errorf("operator -> unbind channel %s from asset %s: %w", channelSourceID, assetID, err)
	}
	return nil
}

// DeleteAssetChannelBindings 清理资产的全部通道绑定
func (r *Dao) DeleteAssetChannelBindings(ctx context.Context, assetID string) error {
	if err := r.db.WithContext(ctx).
		Where("asset_id = ?", assetID).
		Delete(&modules.AssetChannelBinding{}).Error; err != nil {
		return fmt.Errorf("operator -> clear channel bindings for asset %s: %w", assetID, err)
	}
	return nil
}
