// 操作媒介实体持久化: CRUD 与资产绑定.
package dao

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"JuanNiang-Ops/internal/operator/modules"
)

// CreateMediaSource 新建媒介实体 (ID 须由 service 预填; Name 全局唯一冲突报错)
func (r *Dao) CreateMediaSource(ctx context.Context, src *modules.MediaSource) error {
	if err := r.db.WithContext(ctx).Create(src).Error; err != nil {
		return fmt.Errorf("operator -> create media source %s: %w", src.Name, err)
	}
	return nil
}

// GetMediaSource 按 ID 查询媒介实体 (软删后不可见; 未找到返回 gorm.ErrRecordNotFound)
func (r *Dao) GetMediaSource(ctx context.Context, id string) (*modules.MediaSource, error) {
	var src modules.MediaSource
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&src).Error; err != nil {
		return nil, fmt.Errorf("operator -> get media source %s: %w", id, err)
	}
	return &src, nil
}

// ListMediaSources 媒介实体列表 (kind 非空时按类型过滤)
func (r *Dao) ListMediaSources(ctx context.Context, kind modules.MediaKind) ([]modules.MediaSource, error) {
	query := r.db.WithContext(ctx).Model(&modules.MediaSource{})
	if kind != "" {
		query = query.Where("kind = ?", kind)
	}
	var sources []modules.MediaSource
	if err := query.Order("created_at").Find(&sources).Error; err != nil {
		return nil, fmt.Errorf("operator -> list media sources: %w", err)
	}
	return sources, nil
}

// UpdateMediaSource 全量更新 (src 须携带主键, 先 Get 再改, 否则 Save 走 Insert 分支)
func (r *Dao) UpdateMediaSource(ctx context.Context, src *modules.MediaSource) error {
	if err := r.db.WithContext(ctx).Save(src).Error; err != nil {
		return fmt.Errorf("operator -> update media source %s: %w", src.ID, err)
	}
	return nil
}

// DeleteMediaSource 软删媒介实体 (绑定关系由 service 在删除前校验并清理)
func (r *Dao) DeleteMediaSource(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Delete(&modules.MediaSource{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("operator -> delete media source %s: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("operator -> delete media source %s: %w", id, gorm.ErrRecordNotFound)
	}
	return nil
}

// ListMediaSourcesByAsset 资产绑定的全部媒介实体 (两步查询: 绑定行 → IN 查询, 资产数量级小无需 JOIN)
func (r *Dao) ListMediaSourcesByAsset(ctx context.Context, assetID string) ([]modules.MediaSource, error) {
	var bindings []modules.AssetMediaBinding
	if err := r.db.WithContext(ctx).Where("asset_id = ?", assetID).Find(&bindings).Error; err != nil {
		return nil, fmt.Errorf("operator -> list media bindings for asset %s: %w", assetID, err)
	}
	if len(bindings) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(bindings))
	for _, b := range bindings {
		ids = append(ids, b.MediaSourceID)
	}
	var sources []modules.MediaSource
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&sources).Error; err != nil {
		return nil, fmt.Errorf("operator -> load media sources for asset %s: %w", assetID, err)
	}
	return sources, nil
}

// ListAssetIDsByMediaSource 反向查询: 绑定了该媒介的资产 ID 列表 (更新触发重建 / 在用检查)
func (r *Dao) ListAssetIDsByMediaSource(ctx context.Context, mediaSourceID string) ([]string, error) {
	var bindings []modules.AssetMediaBinding
	if err := r.db.WithContext(ctx).Where("media_source_id = ?", mediaSourceID).Find(&bindings).Error; err != nil {
		return nil, fmt.Errorf("operator -> list bindings for media source %s: %w", mediaSourceID, err)
	}
	ids := make([]string, 0, len(bindings))
	for _, b := range bindings {
		ids = append(ids, b.AssetID)
	}
	return ids, nil
}

// BindAssetMedia 绑定媒介到资产 (OnConflict DoNothing 幂等)
func (r *Dao) BindAssetMedia(ctx context.Context, assetID, mediaSourceID string) error {
	binding := modules.AssetMediaBinding{AssetID: assetID, MediaSourceID: mediaSourceID}
	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&binding).Error; err != nil {
		return fmt.Errorf("operator -> bind media %s to asset %s: %w", mediaSourceID, assetID, err)
	}
	return nil
}

// UnbindAssetMedia 解除资产与媒介的绑定 (绑定不存在时 RowsAffected==0, 幂等)
func (r *Dao) UnbindAssetMedia(ctx context.Context, assetID, mediaSourceID string) error {
	if err := r.db.WithContext(ctx).
		Where("asset_id = ? AND media_source_id = ?", assetID, mediaSourceID).
		Delete(&modules.AssetMediaBinding{}).Error; err != nil {
		return fmt.Errorf("operator -> unbind media %s from asset %s: %w", mediaSourceID, assetID, err)
	}
	return nil
}

// DeleteAssetMediaBindings 清理资产的全部媒介绑定 (DeleteAsset 时防孤儿行)
func (r *Dao) DeleteAssetMediaBindings(ctx context.Context, assetID string) error {
	if err := r.db.WithContext(ctx).
		Where("asset_id = ?", assetID).
		Delete(&modules.AssetMediaBinding{}).Error; err != nil {
		return fmt.Errorf("operator -> clear media bindings for asset %s: %w", assetID, err)
	}
	return nil
}
