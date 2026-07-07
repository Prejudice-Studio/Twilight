package api

// embyRestrictedPolicyKeys lists Emby/Jellyfin policy flags that Twilight keeps
// disabled for managed users. These capabilities are not needed for ordinary
// playback access and can grant destructive content, sharing, device-control or
// expensive transcoding powers.
var embyRestrictedPolicyKeys = []string{
	"EnableContentDownloading",
	"EnableContentDeletion",
	"EnableContentDeletionFromFolders",
	"EnableSync",
	"EnableSyncTranscoding",
	"EnableMediaConversion",
	"EnablePublicSharing",
	"EnableRemoteControlOfOtherUsers",
	"EnableSharedDeviceControl",
	"EnableCameraUpload",
	"EnableSubtitleDownloading",
	"EnableSubtitleManagement",
	"EnableLiveTvAccess",
	"EnableLiveTvManagement",
	"EnableVideoPlaybackTranscoding",
	"EnableAudioPlaybackTranscoding",
	"EnablePlaybackRemuxing",
}

func embyHardenManagedPolicy(policy map[string]any) {
	for _, key := range embyRestrictedPolicyKeys {
		policy[key] = false
	}
}

func embyGrantAllLibraryAccess(policy map[string]any) {
	policy["EnableAllFolders"] = true
	policy["EnabledFolders"] = []string{}
	policy["EnableAllChannels"] = true
	policy["EnabledChannels"] = []string{}
}
