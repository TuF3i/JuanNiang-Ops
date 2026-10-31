// 资产实例生命周期: 加载/启用/停用/重建/释放.
// 顺序约定 (与 adapter 同构): 先落库再动内存; 构建失败不回滚落库, 保持未运行等重试.
package service

import (
	"context"
	"fmt"
	"log"
)

// LoadEnabled 加载 DB 中已启用资产并创建实例 (CreateRuntime 调用).
// 单个资产构建失败只记日志不中断 (与 adapter 的驱动装配同语义).
func (r *Service) LoadEnabled() error {
	assets, err := r.dao.ListEnabledAssets(context.Background())
	if err != nil {
		return fmt.Errorf("load enabled assets -> %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range assets {
		asset := assets[i]
		if _, ok := r.instances[asset.ID]; ok {
			continue // 已运行
		}
		r.instances[asset.ID] = buildInstance(asset)
		log.Printf("[operator] asset loaded (id=%s name=%s)", asset.ID, asset.Name)
	}
	return nil
}

// EnableAsset 启用资产: 落库 enabled=true → 创建实例.
// 实例构建不整体失败 (单个媒介/通道失败跳过), 故除 DB 错误外不返回错误.
func (r *Service) EnableAsset(ctx context.Context, id string) error {
	if err := r.dao.SetEnabled(ctx, id, true); err != nil {
		return r.translateNotFound(fmt.Errorf("enable asset -> %w", err))
	}

	asset, err := r.dao.GetAsset(ctx, id)
	if err != nil {
		return r.translateNotFound(fmt.Errorf("enable asset -> %w", err))
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.instances[id]; ok {
		return nil // 已运行
	}
	r.instances[id] = buildInstance(*asset)
	log.Printf("[operator] asset enabled (id=%s name=%s)", asset.ID, asset.Name)
	return nil
}

// DisableAsset 停用资产: 落库 enabled=false → 关闭并移除实例 (幂等)
func (r *Service) DisableAsset(ctx context.Context, id string) error {
	if err := r.dao.SetEnabled(ctx, id, false); err != nil {
		return r.translateNotFound(fmt.Errorf("disable asset -> %w", err))
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if ins, ok := r.instances[id]; ok {
		ins.close()
		delete(r.instances, id)
	}
	log.Printf("[operator] asset disabled (id=%s)", id)
	return nil
}

// reCreate 重建实例 (UpdateAsset 配置变更后调用): 关旧 → 按 DB 最新配置重新构建.
// 停用状态则只移除旧实例.
func (r *Service) reCreate(ctx context.Context, id string) error {
	asset, err := r.dao.GetAsset(ctx, id)
	if err != nil {
		return r.translateNotFound(fmt.Errorf("recreate asset -> %w", err))
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if ins, ok := r.instances[id]; ok {
		ins.close()
		delete(r.instances, id)
	}
	if !asset.Enabled {
		return nil
	}
	r.instances[id] = buildInstance(*asset)
	log.Printf("[operator] asset recreated (id=%s name=%s)", asset.ID, asset.Name)
	return nil
}

// Release 释放全部实例 (幂等; 由 runtime.ReleaseRuntime 调用)
func (r *Service) Release() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, ins := range r.instances {
		ins.close()
		delete(r.instances, id)
	}
	log.Printf("[operator] service released")
	return nil
}

// translateNotFound 将 dao 层的 gorm.ErrRecordNotFound 翻译为 sentinel ErrAssetNotFound
func (r *Service) translateNotFound(err error) error {
	if isNotFound(err) {
		return fmt.Errorf("%w: %s", ErrAssetNotFound, err)
	}
	return err
}
