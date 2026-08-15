package midi

/*
#cgo LDFLAGS: -framework CoreMIDI -framework CoreFoundation

#include <CoreMIDI/CoreMIDI.h>
#include <CoreFoundation/CoreFoundation.h>
#include <pthread.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

typedef struct xt_virtual_message {
	uint8_t *bytes;
	uint32_t length;
	struct xt_virtual_message *next;
} xt_virtual_message;

typedef struct {
	MIDIClientRef client;
	MIDIEndpointRef source;
	MIDIEndpointRef destination;
	pthread_mutex_t mutex;
	xt_virtual_message *head;
	xt_virtual_message *tail;
} xt_virtual_state;

static void xt_virtual_read(const MIDIPacketList *packets, void *read_ref, void *source_ref) {
	(void)source_ref;
	xt_virtual_state *state = (xt_virtual_state *)read_ref;
	if (state == NULL || packets == NULL) {
		return;
	}
	const MIDIPacket *packet = &packets->packet[0];
	for (UInt32 index = 0; index < packets->numPackets; index++) {
		if (packet->length > 0) {
			xt_virtual_message *message = (xt_virtual_message *)calloc(1, sizeof(xt_virtual_message));
			if (message != NULL) {
				message->bytes = (uint8_t *)malloc(packet->length);
				if (message->bytes != NULL) {
					memcpy(message->bytes, packet->data, packet->length);
					message->length = packet->length;
					pthread_mutex_lock(&state->mutex);
					if (state->tail == NULL) {
						state->head = message;
					} else {
						state->tail->next = message;
					}
					state->tail = message;
					pthread_mutex_unlock(&state->mutex);
				} else {
					free(message);
				}
			}
		}
		packet = MIDIPacketNext(packet);
	}
}

static void xt_set_string_property(MIDIObjectRef object, CFStringRef property, CFStringRef value) {
	if (object != 0 && value != NULL) {
		MIDIObjectSetStringProperty(object, property, value);
	}
}

static xt_virtual_state *xt_virtual_open(const char *name, OSStatus *result) {
	if (name == NULL) {
		*result = -50;
		return NULL;
	}
	xt_virtual_state *state = (xt_virtual_state *)calloc(1, sizeof(xt_virtual_state));
	if (state == NULL) {
		*result = -108;
		return NULL;
	}
	pthread_mutex_init(&state->mutex, NULL);
	CFStringRef endpoint_name = CFStringCreateWithCString(NULL, name, kCFStringEncodingUTF8);
	CFStringRef client_name = CFStringCreateWithCString(NULL, "xtouch-cli simulator", kCFStringEncodingUTF8);
	CFStringRef manufacturer = CFStringCreateWithCString(NULL, "misofm", kCFStringEncodingUTF8);
	CFStringRef model = CFStringCreateWithCString(NULL, "X-Touch Simulator", kCFStringEncodingUTF8);
	if (endpoint_name == NULL || client_name == NULL || manufacturer == NULL || model == NULL) {
		*result = -108;
		goto fail;
	}
	*result = MIDIClientCreate(client_name, NULL, NULL, &state->client);
	if (*result != noErr) {
		goto fail;
	}
	*result = MIDISourceCreate(state->client, endpoint_name, &state->source);
	if (*result != noErr) {
		goto fail;
	}
	*result = MIDIDestinationCreate(state->client, endpoint_name, xt_virtual_read, state, &state->destination);
	if (*result != noErr) {
		goto fail;
	}
	xt_set_string_property(state->source, kMIDIPropertyManufacturer, manufacturer);
	xt_set_string_property(state->source, kMIDIPropertyModel, model);
	xt_set_string_property(state->destination, kMIDIPropertyManufacturer, manufacturer);
	xt_set_string_property(state->destination, kMIDIPropertyModel, model);
	CFRelease(endpoint_name);
	CFRelease(client_name);
	CFRelease(manufacturer);
	CFRelease(model);
	return state;

fail:
	if (state->destination != 0) MIDIEndpointDispose(state->destination);
	if (state->source != 0) MIDIEndpointDispose(state->source);
	if (state->client != 0) MIDIClientDispose(state->client);
	if (endpoint_name != NULL) CFRelease(endpoint_name);
	if (client_name != NULL) CFRelease(client_name);
	if (manufacturer != NULL) CFRelease(manufacturer);
	if (model != NULL) CFRelease(model);
	pthread_mutex_destroy(&state->mutex);
	free(state);
	return NULL;
}

static int32_t xt_virtual_next(xt_virtual_state *state, uint8_t *buffer, uint32_t capacity) {
	if (state == NULL || buffer == NULL || capacity == 0) {
		return -1;
	}
	pthread_mutex_lock(&state->mutex);
	xt_virtual_message *message = state->head;
	if (message == NULL) {
		pthread_mutex_unlock(&state->mutex);
		return 0;
	}
	state->head = message->next;
	if (state->head == NULL) {
		state->tail = NULL;
	}
	pthread_mutex_unlock(&state->mutex);
	int32_t result = (int32_t)message->length;
	if (message->length <= capacity) {
		memcpy(buffer, message->bytes, message->length);
	} else {
		result = -result;
	}
	free(message->bytes);
	free(message);
	return result;
}

static OSStatus xt_virtual_send(xt_virtual_state *state, const uint8_t *bytes, uint32_t length) {
	if (state == NULL || state->source == 0 || bytes == NULL || length == 0) {
		return -50;
	}
	ByteCount capacity = sizeof(MIDIPacketList) + length + 256;
	MIDIPacketList *packets = (MIDIPacketList *)calloc(1, capacity);
	if (packets == NULL) {
		return -108;
	}
	MIDIPacket *packet = MIDIPacketListInit(packets);
	packet = MIDIPacketListAdd(packets, capacity, packet, 0, length, bytes);
	if (packet == NULL) {
		free(packets);
		return kMIDIMessageSendErr;
	}
	OSStatus result = MIDIReceived(state->source, packets);
	free(packets);
	return result;
}

static void xt_virtual_close(xt_virtual_state *state) {
	if (state == NULL) {
		return;
	}
	if (state->destination != 0) MIDIEndpointDispose(state->destination);
	if (state->source != 0) MIDIEndpointDispose(state->source);
	if (state->client != 0) MIDIClientDispose(state->client);
	pthread_mutex_lock(&state->mutex);
	xt_virtual_message *message = state->head;
	while (message != NULL) {
		xt_virtual_message *next = message->next;
		free(message->bytes);
		free(message);
		message = next;
	}
	state->head = NULL;
	state->tail = NULL;
	pthread_mutex_unlock(&state->mutex);
	pthread_mutex_destroy(&state->mutex);
	free(state);
}
*/
import "C"

import (
	"fmt"
	"runtime"
	"sync"
	"time"
	"unsafe"
)

const virtualMessageCapacity = 4096

type coreMIDIVirtualDevice struct {
	name      string
	state     *C.xt_virtual_state
	received  chan []byte
	done      chan struct{}
	closeOnce sync.Once
	waitGroup sync.WaitGroup
	mutex     sync.Mutex
}

func OpenVirtualDevice(name string) (VirtualDevice, error) {
	if name == "" {
		return nil, fmt.Errorf("virtual MIDI device name cannot be empty")
	}
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	var status C.OSStatus
	state := C.xt_virtual_open(cName, &status)
	if state == nil || status != 0 {
		return nil, fmt.Errorf("create CoreMIDI virtual X-Touch: OSStatus %d", int32(status))
	}
	device := &coreMIDIVirtualDevice{
		name: name, state: state, received: make(chan []byte, 256), done: make(chan struct{}),
	}
	device.waitGroup.Add(1)
	go device.poll()
	return device, nil
}

func (device *coreMIDIVirtualDevice) Name() string { return device.name }

func (device *coreMIDIVirtualDevice) Received() <-chan []byte { return device.received }

func (device *coreMIDIVirtualDevice) Send(message []byte) error {
	if len(message) == 0 {
		return fmt.Errorf("cannot send an empty virtual MIDI message")
	}
	device.mutex.Lock()
	defer device.mutex.Unlock()
	if device.state == nil {
		return fmt.Errorf("virtual MIDI device is closed")
	}
	status := C.xt_virtual_send(device.state, (*C.uint8_t)(unsafe.Pointer(&message[0])), C.uint32_t(len(message)))
	runtime.KeepAlive(message)
	if status != 0 {
		return fmt.Errorf("send virtual MIDI message: OSStatus %d", int32(status))
	}
	return nil
}

func (device *coreMIDIVirtualDevice) Close() error {
	device.closeOnce.Do(func() {
		close(device.done)
		device.waitGroup.Wait()
		device.mutex.Lock()
		if device.state != nil {
			C.xt_virtual_close(device.state)
			device.state = nil
		}
		device.mutex.Unlock()
		close(device.received)
	})
	return nil
}

func (device *coreMIDIVirtualDevice) poll() {
	defer device.waitGroup.Done()
	ticker := time.NewTicker(2 * time.Millisecond)
	defer ticker.Stop()
	buffer := make([]byte, virtualMessageCapacity)
	for {
		select {
		case <-device.done:
			return
		case <-ticker.C:
			for {
				length := int(C.xt_virtual_next(device.state, (*C.uint8_t)(unsafe.Pointer(&buffer[0])), C.uint32_t(len(buffer))))
				if length == 0 {
					break
				}
				if length < 0 {
					continue
				}
				message := append([]byte(nil), buffer[:length]...)
				select {
				case device.received <- message:
				case <-device.done:
					return
				}
			}
		}
	}
}
