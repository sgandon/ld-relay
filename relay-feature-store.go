package main

import (
	"encoding/json"
	es "github.com/launchdarkly/eventsource"
	ld "gopkg.in/launchdarkly/go-client.v3"
	"time"
)

type SSERelayFeatureStore struct {
	store          ld.FeatureStore
	allPublisher   *es.Server
	flagsPublisher *es.Server
	apiKey         string
}

type allRepository struct {
	relayStore *SSERelayFeatureStore
}
type flagsRepository struct {
	relayStore *SSERelayFeatureStore
}

func NewSSERelayFeatureStore(apiKey string, allPublisher *es.Server, flagsPublisher *es.Server, baseFeatureStore ld.FeatureStore, heartbeatInterval int) *SSERelayFeatureStore {
	relayStore := &SSERelayFeatureStore{
		store:          baseFeatureStore,
		apiKey:         apiKey,
		allPublisher:   allPublisher,
		flagsPublisher: flagsPublisher,
	}

	allPublisher.Register(apiKey, allRepository{relayStore})
	flagsPublisher.Register(apiKey, flagsRepository{relayStore})

	if heartbeatInterval > 0 {
		go func() {
			t := time.NewTicker(time.Duration(heartbeatInterval) * time.Second)
			for {
				relayStore.heartbeat()
				<-t.C
			}
		}()
	}

	return relayStore
}

func (relay *SSERelayFeatureStore) keys() []string {
	return []string{relay.apiKey}
}

func (relay *SSERelayFeatureStore) heartbeat() {
	relay.allPublisher.Publish(relay.keys(), heartbeatEvent("hb"))
	relay.flagsPublisher.Publish(relay.keys(), heartbeatEvent("hb"))
}

func (relay *SSERelayFeatureStore) Get(kind ld.VersionedDataKind, key string) (ld.VersionedData, error) {
	return relay.store.Get(kind, key)
}

func (relay *SSERelayFeatureStore) All(kind ld.VersionedDataKind) (map[string]ld.VersionedData, error) {
	return relay.store.All(kind)
}

func (relay *SSERelayFeatureStore) Init(allData map[ld.VersionedDataKind]map[string]ld.VersionedData) error {
	err := relay.store.Init(allData)

	if err != nil {
		return err
	}

	relay.allPublisher.Publish(relay.keys(), makePutEvent(allData[ld.Features], allData[ld.Segments]))
	relay.flagsPublisher.Publish(relay.keys(), makeFlagsPutEvent(allData[ld.Features]))

	return nil
}

func (relay *SSERelayFeatureStore) Delete(kind ld.VersionedDataKind, key string, version int) error {
	err := relay.store.Delete(kind, key, version)
	if err != nil {
		return err
	}

	relay.allPublisher.Publish(relay.keys(), makeDeleteEvent(kind, key, version))
	if kind == ld.Features {
		relay.flagsPublisher.Publish(relay.keys(), makeFlagsDeleteEvent(key, version))
	}

	return nil
}

func (relay *SSERelayFeatureStore) Upsert(kind ld.VersionedDataKind, item ld.VersionedData) error {
	err := relay.store.Upsert(kind, item)

	if err != nil {
		return err
	}

	newItem, err := relay.store.Get(kind, item.GetKey())

	if err != nil {
		return err
	}

	if newItem != nil {
		relay.allPublisher.Publish(relay.keys(), makeUpsertEvent(kind, newItem))
		if kind == ld.Features {
			relay.flagsPublisher.Publish(relay.keys(), makeFlagsUpsertEvent(newItem))
		}
	}

	return nil
}

func (relay *SSERelayFeatureStore) Initialized() bool {
	return relay.store.Initialized()
}

// Allows the feature store to act as an SSE repository (to send bootstrap events)
func (r flagsRepository) Replay(channel, id string) (out chan es.Event) {
	out = make(chan es.Event)
	go func() {
		defer close(out)
		if r.relayStore.Initialized() {
			flags, err := r.relayStore.All(ld.Features)

			if err != nil {
				Error.Printf("Error getting all flags: %s\n", err.Error())
			} else {
				out <- makeFlagsPutEvent(flags)
			}
		}
	}()
	return
}

func (r allRepository) Replay(channel, id string) (out chan es.Event) {
	out = make(chan es.Event)
	go func() {
		defer close(out)
		if r.relayStore.Initialized() {
			flags, err := r.relayStore.All(ld.Features)

			if err != nil {
				Error.Printf("Error getting all flags: %s\n", err.Error())
			} else {
				segments, err := r.relayStore.All(ld.Segments)
				if err != nil {
					Error.Printf("Error getting all segments: %s\n", err.Error())
				} else {
					out <- makePutEvent(flags, segments)
				}
			}

		}
	}()
	return
}

var dataKindApiName = map[ld.VersionedDataKind]string{
	ld.Features: "flags",
	ld.Segments: "segments",
}

type flagsPutEvent map[string]ld.VersionedData
type allPutEvent map[string]map[string]ld.VersionedData

type deleteEvent struct {
	Path    string `json:"path"`
	Version int    `json:"version"`
}

type upsertEvent struct {
	Path string           `json:"path"`
	D    ld.VersionedData `json:"data"`
}

type heartbeatEvent string

func (h heartbeatEvent) Id() string {
	return ""
}

func (h heartbeatEvent) Event() string {
	return ""
}

func (h heartbeatEvent) Data() string {
	return ""
}

func (h heartbeatEvent) Comment() string {
	return string(h)
}

func (t flagsPutEvent) Id() string {
	return ""
}

func (t flagsPutEvent) Event() string {
	return "put"
}

func (t flagsPutEvent) Data() string {
	data, _ := json.Marshal(t)

	return string(data)
}

func (t flagsPutEvent) Comment() string {
	return ""
}

func (t allPutEvent) Id() string {
	return ""
}

func (t allPutEvent) Event() string {
	return "put"
}

func (t allPutEvent) Data() string {
	data, _ := json.Marshal(t)

	return string(data)
}

func (t allPutEvent) Comment() string {
	return ""
}

func (t upsertEvent) Id() string {
	return ""
}

func (t upsertEvent) Event() string {
	return "patch"
}

func (t upsertEvent) Data() string {
	data, _ := json.Marshal(t)

	return string(data)
}

func (t upsertEvent) Comment() string {
	return ""
}

func (t deleteEvent) Id() string {
	return ""
}

func (t deleteEvent) Event() string {
	return "delete"
}

func (t deleteEvent) Data() string {
	data, _ := json.Marshal(t)

	return string(data)
}

func (t deleteEvent) Comment() string {
	return ""
}

func makeUpsertEvent(kind ld.VersionedDataKind, item ld.VersionedData) es.Event {
	return upsertEvent{
		Path: "/" + dataKindApiName[kind] + "/" + item.GetKey(),
		D:    item,
	}
}

func makeFlagsUpsertEvent(item ld.VersionedData) es.Event {
	return upsertEvent{
		Path: "/" + item.GetKey(),
		D:    item,
	}
}

func makeDeleteEvent(kind ld.VersionedDataKind, key string, version int) es.Event {
	return deleteEvent{
		Path:    "/" + dataKindApiName[kind] + "/" + key,
		Version: version,
	}
}

func makeFlagsDeleteEvent(key string, version int) es.Event {
	return deleteEvent{
		Path:    "/" + key,
		Version: version,
	}
}

func makePutEvent(flags map[string]ld.VersionedData, segments map[string]ld.VersionedData) es.Event {
	var allData = map[string]map[string]ld.VersionedData{
		"flags":    {},
		"segments": {},
	}
	for key, flag := range flags {
		allData["flags"][key] = flag
	}
	for key, seg := range segments {
		allData["segments"][key] = seg
	}
	return allPutEvent(allData)
}

func makeFlagsPutEvent(flags map[string]ld.VersionedData) es.Event {
	return flagsPutEvent(flags)
}
