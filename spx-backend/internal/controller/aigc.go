package controller

import (
	"context"
	"net"
	"net/http"
	"net/url"

	"github.com/goplus/builder/spx-backend/internal/log"
	"github.com/goplus/builder/spx-backend/internal/model"
)

type MattingParams struct {
	// ImageUrl is the image URL to be matted.
	ImageUrl string `json:"imageUrl"`
}

type GenerateParams struct {
	// Category is the category of the image to be generated.
	Category []string `json:"category"`
	// Keyword is the keyword of the image to be generated.
	Keyword string `json:"keyword"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
}

type GetGenerateParams struct {
	Category string `json:"category"`
	Prompt   string `json:"prompt"`
}

type GetGenerateResult struct {
	ImageUrl string `json:"image_url"`
}

type GenerateResult struct {
	ImageJobId string `json:"imageJobId"`
}

type GenerateSpriteParams struct {
	// ImageUrl is the image URL to be generated as sprite.
	ImageUrl string `json:"image_url"`
}

type GetGenerateSpriteParams struct {
	ImageUrl string `json:"image_url"`
}

type GenerateSpriteResult struct {
	SpriteUrl string `json:"material_url"`
}

type GetEmbeddingParams struct {
	Prompt      string `json:"prompt"`
	CallbackUrl string `json:"callback_url"`
}

type GetEmbeddingResult struct {
	Embedding []float32 `json:"embedding"`
	Desc      string    `json:"desc"`
}

type GetAIAssetStatusResult struct {
	// Status is the status of the AI asset.
	Status AssetStatus    `json:"status"`
	Result AIStatusResult `json:"result"`
}

type AssetStatus int

const (
	waiting    AssetStatus = iota
	generating             // 正在生成
	finish                 // 已完成
)

type AIStatusResult struct {
	JobId string          `json:"jobId"`
	Type  model.AssetType `json:"type"`
	Files AIStatusFiles   `json:"files"`
}

type AIStatusFiles struct {
	ImageUrl    string `json:"imageUrl"`
	SkeletonUrl string `json:"skeletonUrl"`
}

func (p *MattingParams) Validate() (ok bool, msg string) {
	if p.ImageUrl == "" {
		return false, "missing imageUrl"
	}

	// It may introduce security risk if we allow arbitrary image URL.
	// Urls targeting local or private network should be rejected.

	url, err := url.Parse(p.ImageUrl)
	if err != nil || url.Host == "" {
		return false, "invalid imageUrl"
	}
	if url.Scheme != "http" && url.Scheme != "https" {
		return false, "invalid imageUrl: unsupported scheme"
	}

	hostname := url.Hostname()
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return false, "invalid imageUrl: lookup IP failed"
	}

	for _, ip := range ips {
		if isIPPrivate(ip) {
			return false, "invalid imageUrl: private IP"
		}
	}

	return true, ""
}

func isIPPrivate(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() {
		return true
	}
	return false
}

type MattingResult struct {
	// ImageUrl is the image URL that has been matted.
	ImageUrl string `json:"imageUrl"`
}

// Matting removes background of given image.
func (ctrl *Controller) Matting(ctx context.Context, params *MattingParams) (*MattingResult, error) {
	logger := log.GetReqLogger(ctx)
	aigcParams := struct {
		ImageUrl string `json:"image_url"`
	}{
		ImageUrl: params.ImageUrl,
	}
	var aigcResult struct {
		ImageUrl string `json:"image_url"`
	}
	err := ctrl.aigcClient.Call(ctx, http.MethodPost, "/matting", &aigcParams, &aigcResult)
	if err != nil {
		logger.Printf("failed to call: %v", err)
		return nil, err
	}
	return &MattingResult{
		ImageUrl: aigcResult.ImageUrl,
	}, nil
}

// Generating follow parameters to generating images.
func (ctrl *Controller) Generating(ctx context.Context, param *GenerateParams) (*GenerateResult, error) {
	logger := log.GetReqLogger(ctx)
	var assetType model.AssetType
	if param.Height > 0 && param.Width > 0 {
		assetType = model.AssetTypeBackdrop
	} else {
		assetType = model.AssetTypeSprite
	}
	newAIAsset, err := model.AddAsset(ctx, ctrl.db, &model.Asset{
		AssetType: assetType, //TODO: it like this have a bug.
	})
	if err != nil {
		logger.Printf("failed to add asset: %v", err)
		return nil, err
	}
	go func(ctx context.Context) {
		var generateResult GetGenerateResult
		err = ctrl.aigcClient.Call(ctx, http.MethodPost, "/generate", &GetGenerateParams{
			Category: StringArrayToString(param.Category), // different separator
			Prompt:   param.Keyword,                       // todo: more parameters
		}, &generateResult)
		if err != nil {
			logger.Printf("failed to call: %v", err)
		}
		_, err = model.UpdateAssetByID(ctx, ctrl.db, newAIAsset.ID, &model.Asset{
			FilesHash: generateResult.ImageUrl,
		})
		if err != nil {
			logger.Printf("failed to update asset: %v", err)
		}
	}(context.Background())

	return &GenerateResult{
		ImageJobId: newAIAsset.ID,
	}, nil
}

// GenerateSprite follow parameters to generating sprite.
func (ctrl *Controller) GenerateSprite(ctx context.Context, param *GenerateSpriteParams) (*GenerateSpriteResult, error) {
	logger := log.GetReqLogger(ctx)
	var generateSpriteResult GenerateSpriteResult
	err := ctrl.aigcClient.Call(ctx, http.MethodPost, "/animate", &GetGenerateSpriteParams{
		ImageUrl: param.ImageUrl,
	}, &generateSpriteResult)
	if err != nil {
		logger.Printf("failed to call: %v", err)
		return nil, err
	}
	return &generateSpriteResult, nil
}

// GetEmbedding get text embedding.
func (ctrl *Controller) GetEmbedding(ctx context.Context, param *GetEmbeddingParams) (*GetEmbeddingResult, error) {
	logger := log.GetReqLogger(ctx)
	var embeddingResult GetEmbeddingResult
	err := ctrl.aigcClient.Call(ctx, http.MethodPost, "/embedding", &param, &embeddingResult)
	if err != nil {
		logger.Printf("failed to call: %v", err)
		return nil, err
	}
	return &embeddingResult, nil
}

// GetAIAssetStatus get AI asset status.
func (ctrl *Controller) GetAIAssetStatus(ctx context.Context, id string) (*GetAIAssetStatusResult, error) {
	logger := log.GetReqLogger(ctx)
	result, err := model.CheckAssetFilesHashByID(ctx, ctrl.ormDb, id)
	if err != nil {
		logger.Printf("failed to get asset: %v", err)
		return nil, err
	}
	var status AssetStatus
	if result.FilesHash == "" {
		status = generating
	} else {
		status = finish
	}
	return &GetAIAssetStatusResult{
		Status: status,
		Result: AIStatusResult{
			JobId: id,
			Type:  result.AssetType,
			Files: AIStatusFiles{
				ImageUrl: result.FilesHash,
			},
		},
	}, nil
}
