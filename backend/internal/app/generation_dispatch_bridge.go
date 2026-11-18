package app

import (
	"context"
	"time"

	"luckytv/backend/internal/generation"
	"luckytv/backend/internal/model"
	"luckytv/backend/internal/protocol"
)

func runImageTask(ctx context.Context, input canvasGenerationInput) (map[string]interface{}, error) {
	return generation.RunImageTask(ctx, input)
}
func imageDataURLs(payload imageResponse) ([]map[string]string, error) {
	return generation.ImageDataURLs(payload)
}
func runTextTask(ctx context.Context, input canvasGenerationInput) (map[string]interface{}, error) {
	return generation.RunTextTask(ctx, input)
}

type (
	streamingAgentToolCall   = generation.StreamingAgentToolCall
	streamingAgentParser     = generation.StreamingAgentParser
	providerTextResult       = generation.ProviderTextResult
	streamingTextDeltaParser = generation.StreamingTextDeltaParser
	videoPollPolicy          = generation.VideoPollPolicy
	videoPollEvent           = generation.VideoPollEvent
	videoPollOutcome         = generation.VideoPollOutcome
	videoDownloadError       = generation.VideoDownloadError
	luckyTVSeedanceMediaReader = generation.LuckyTVSeedanceMediaReader
	luckyTVSeedanceUploadSession = generation.LuckyTVSeedanceUploadSession
	luckyTVSeedanceUploadComplete = generation.LuckyTVSeedanceUploadComplete
	luckyTVSeedanceUploadRequest = generation.LuckyTVSeedanceUploadRequest
	ArkPrivateAssetSyncResult = generation.ArkPrivateAssetSyncResult
	seedanceVideosRequest    = generation.SeedanceVideosRequest
	geminiImageRequest       = generation.GeminiImageRequest
	geminiImageContent       = generation.GeminiImageContent
	geminiImageContentPart   = generation.GeminiImageContentPart
	geminiImageInlineData    = generation.GeminiImageInlineData
	geminiImageGenerationConfig = generation.GeminiImageGenerationConfig
	geminiImageConfig        = generation.GeminiImageConfig
	seedanceAgentPlanRequest = generation.SeedanceAgentPlanRequest
)

func restoreLuckyTVSeedanceAudioControl(ctx context.Context, config providerConfig, video *VideoCapabilityConfig) {
	generation.RestoreLuckyTVSeedanceAudioControl(ctx, config, video)
}
func runVideoTask(ctx context.Context, input canvasGenerationInput) (map[string]interface{}, error) {
	return generation.RunVideoTask(ctx, input)
}
func runAudioTask(ctx context.Context, input canvasGenerationInput) (map[string]interface{}, error) {
	return generation.RunAudioTask(ctx, input)
}
func decodeProviderDataURL(value string) (string, []byte, error) {
	return generation.DecodeProviderDataURL(value)
}
func queryProtocolAdapterVideoTask(ctx context.Context, input canvasGenerationInput, adapter protocol.Adapter, taskID string) (map[string]interface{}, string, error) {
	return generation.QueryProtocolAdapterVideoTask(ctx, input, adapter, taskID)
}
func validateGenerationInterface(mode string, interfaceType string) error {
	return generation.ValidateGenerationInterface(mode, interfaceType)
}
func validateGenerationInterfaceWithRegistry(registry *protocol.Registry, mode string, interfaceType string) error {
	return generation.ValidateGenerationInterfaceWithRegistry(registry, mode, interfaceType)
}
func defaultVideoPollPolicy() videoPollPolicy { return generation.DefaultVideoPollPolicy() }
func runVideoPollLoop(ctx context.Context, taskID string, policy videoPollPolicy, query func(context.Context) (videoPollOutcome, error)) (map[string]interface{}, error) {
	return generation.RunVideoPollLoop(ctx, taskID, policy, query)
}
func runVideoDownload(ctx context.Context, taskID string, policy videoPollPolicy, download func(context.Context) ([]byte, string, error)) ([]byte, string, error) {
	return generation.RunVideoDownload(ctx, taskID, policy, download)
}
func retryableVideoPollError(ctx context.Context, err error) (bool, bool) {
	return generation.RetryableVideoPollError(ctx, err)
}
func isLuckyTVVideoConfig(ctx context.Context, config providerConfig) bool {
	return generation.IsLuckyTVVideoConfig(ctx, config)
}
func isLuckyTVSeedancePreuploadConfig(ctx context.Context, config providerConfig) bool {
	return generation.IsLuckyTVSeedancePreuploadConfig(ctx, config)
}
func isArkPlanVideoConfig(config providerConfig) bool { return generation.IsArkPlanVideoConfig(config) }
func parseBool(value string, fallback bool) bool      { return generation.ParseBool(value, fallback) }
func sleepContext(ctx context.Context, duration time.Duration) error {
	return generation.SleepContext(ctx, duration)
}
func prepareLuckyTVSeedanceReferences(ctx context.Context, config providerConfig, input *canvasGenerationInput, read luckyTVSeedanceMediaReader) error {
	return generation.PrepareLuckyTVVideoReferences(ctx, config, input, read)
}
func skipLuckyTVSeedanceMedia(media providerMedia) bool {
	return generation.SkipLuckyTVSeedanceMedia(media)
}
func luckyTVSeedanceKindMaxBytes(kind string) int64 {
	return generation.LuckyTVSeedanceKindMaxBytes(kind)
}
func validateLuckyTVSeedanceMediaSize(kind string, size int64) error {
	return generation.ValidateLuckyTVSeedanceMediaSize(kind, size)
}
func withTaskExecutionID(ctx context.Context, taskID string) context.Context {
	return generation.WithTaskExecutionID(ctx, taskID)
}
func taskExecutionID(ctx context.Context) string { return generation.TaskExecutionID(ctx) }
func withoutProviderAnalytics(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, providerAnalyticsKey{}, providerAnalyticsContext{})
	return generation.WithoutCallAccounting(ctx)
}
func isArkPrivateAssetVideoConfig(config providerConfig) bool {
	return generation.IsArkPrivateAssetVideoConfig(config)
}
func arkPrivateAssetAutomaticSyncEnabled(setting arkPrivateAssetSettingValue) (bool, error) {
	return generation.ArkPrivateAssetAutomaticSyncEnabled(setting)
}
func arkPrivateAssetResourceID(reference providerMedia) string {
	return generation.ArkPrivateAssetResourceID(reference)
}

var arkPrivateAssetTypeForMime = generation.ArkPrivateAssetTypeForMime

func shouldRetryArkPrivateAssetBinding(binding *model.ArkPrivateAssetBinding) bool {
	return generation.ShouldRetryArkPrivateAssetBinding(binding)
}
func shouldResumeArkPrivateAssetPolling(binding *model.ArkPrivateAssetBinding) bool {
	return generation.ShouldResumeArkPrivateAssetPolling(binding)
}
func callArkPrivateAssetAPI(ctx context.Context, setting arkPrivateAssetSettingValue, action string, payload map[string]interface{}) (map[string]interface{}, error) {
	return generation.CallArkPrivateAssetAPI(ctx, setting, action, payload)
}
func arkPrivateAssetResponseField(response map[string]interface{}, keys ...string) string {
	return generation.ArkPrivateAssetResponseField(response, keys...)
}
