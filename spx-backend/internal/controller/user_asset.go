package controller

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/goplus/builder/spx-backend/internal/log"
	"github.com/goplus/builder/spx-backend/internal/model"
)

// AddUserAssetParams holds parameters for adding an user asset.
type AddUserAssetParams struct {
	// AssetID is the identifier for the asset.
	AssetID string `json:"assetId"`
}

// AddUserAsset adds an asset.
func (ctrl *Controller) AddUserAsset(ctx context.Context, params *AddUserAssetParams, assetType string, owner string) error {
	logger := log.GetReqLogger(ctx)
	fmt.Println("AddUserAsset, assetType: ", assetType)
	assetId, err := strconv.Atoi(params.AssetID)
	_, err = model.AddUserAsset(ctx, ctrl.ormDb, &model.UserAsset{
		Owner:             owner,
		AssetID:           assetId,
		RelationType:      model.RelationType(assetType),
		RelationTimestamp: time.Now(),
	})
	if err != nil {
		logger.Printf("failed to add asset: %v", err)
		return err
	}

	return nil
}

// ListUserAssets lists assets for a specific user with various filter and sort options.
func (ctrl *Controller) ListUserAssets(ctx context.Context, assetType string, params *ListAssetsParams) (*model.ByPage[model.Asset], error) {
	logger := log.GetReqLogger(ctx)

	// Initialize the filter conditions
	var wheres []model.FilterCondition
	wheres = append(wheres, model.FilterCondition{Column: "ua.owner", Operation: "=", Value: *params.Owner}, model.FilterCondition{Column: "ua.relation_type", Operation: "=", Value: assetType})

	// Apply additional filters based on parameters
	if params.Keyword != "" {
		wheres = append(wheres, model.FilterCondition{Column: "a.display_name", Operation: "LIKE", Value: "%" + params.Keyword + "%"})
	}
	if params.AssetType != nil {
		wheres = append(wheres, model.FilterCondition{Column: "a.asset_type", Operation: "=", Value: *params.AssetType})
	}
	// Define order conditions based on input
	var orders []model.OrderByCondition
	switch params.OrderBy {
	case TimeDesc:
		orders = append(orders, model.OrderByCondition{Column: "a.c_time", Direction: "DESC"})
	case ClickCountDesc:
		orders = append(orders, model.OrderByCondition{Column: "a.click_count", Direction: "DESC"})
	}

	// Use the QueryByPage function to get paginated results
	query := `
		SELECT a.*
		FROM asset a
		JOIN user_asset ua ON a.id = ua.asset_id
	`

	assets, err := model.ListUserAssets(ctx, ctrl.db, params.Pagination, wheres, orders, query)
	if err != nil {
		logger.Printf("failed to list user assets: %v", err)
		return nil, err
	}

	return assets, nil
}

// DeleteUserAsset deletes an asset.
func (ctrl *Controller) DeleteUserAsset(ctx context.Context, assetType string, assetID string, owner string) error {
	logger := log.GetReqLogger(ctx)

	// Delete the user asset
	err := model.DeleteUserAsset(ctx, ctrl.ormDb, assetID, model.RelationType(assetType), owner)
	if err != nil {
		logger.Printf("failed to delete user asset: %v", err)
		return err
	}

	return nil
}
