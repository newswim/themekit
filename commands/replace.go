package commands

import (
	"fmt"
	"github.com/Shopify/themekit"
	"os"
)

type ReplaceOptions struct {
	BasicOptions
	Bucket *themekit.LeakyBucket
}

func ReplaceCommand(args map[string]interface{}) chan bool {
	options := ReplaceOptions{}
	extractThemeClient(&options.Client, args)
	extractEventLog(&options.EventLog, args)
	options.Filenames = extractStringSlice("filenames", args)

	return Replace(options)
}

func Replace(options ReplaceOptions) chan bool {
	rawEvents, throttledEvents := prepareChannel(options)
	done, logs := options.Client.Process(throttledEvents)
	mergeEvents(options.getEventLog(), []chan themekit.ThemeEvent{logs})
	enqueueEvents(options, rawEvents)
	return done
}

func enqueueEvents(options ReplaceOptions, events chan themekit.AssetEvent) {
	client := options.Client
	filenames := options.Filenames

	root, _ := os.Getwd()
	if len(filenames) == 0 {
		logDebug(fmt.Sprintf("Selecting all valid theme files from within '%s'", root), options.EventLog)
		go fullReplace(client.AssetListSyncWithFields([]string{themekit.AssetFieldKey}), client.LocalAssets(root), events, options)
		return
	}
	go func() {
		for _, filename := range filenames {
			asset, err := themekit.LoadAsset(root, filename)
			if err == nil {
				events <- themekit.NewUploadEvent(asset)
			}
		}
		close(events)
	}()
}

func fullReplace(remoteAssets, localAssets []themekit.Asset, events chan themekit.AssetEvent, options ReplaceOptions) {
	logDebug(fmt.Sprintf("Retrieved %d assets from API: %s", len(remoteAssets), themekit.WhiteText(remoteAssets)), options.EventLog)
	logDebug(fmt.Sprintf("Retrieved %d files from disk: %s", len(localAssets), themekit.WhiteText(localAssets)), options.EventLog)

	assetsActions := map[string]themekit.AssetEvent{}
	generateActions := func(assets []themekit.Asset, assetEventFn func(asset themekit.Asset) themekit.SimpleAssetEvent) {
		for _, asset := range assets {
			assetsActions[asset.Key] = assetEventFn(asset)
		}
	}
	generateActions(remoteAssets, themekit.NewRemovalEvent)
	generateActions(localAssets, themekit.NewUploadEvent)
	go func() {
		for _, event := range assetsActions {
			events <- event
		}
		close(events)
	}()

}

func prepareChannel(options ReplaceOptions) (rawEvents, throttledEvents chan themekit.AssetEvent) {
	rawEvents = make(chan themekit.AssetEvent)
	if options.Bucket == nil {
		return rawEvents, rawEvents
	}

	foreman := themekit.NewForeman(options.Bucket)
	foreman.JobQueue = rawEvents
	foreman.WorkerQueue = make(chan themekit.AssetEvent)
	foreman.IssueWork()
	return foreman.JobQueue, foreman.WorkerQueue
}
