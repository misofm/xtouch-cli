package midi

/*
#cgo LDFLAGS: -framework CoreMIDI -framework CoreFoundation

#include <CoreMIDI/CoreMIDI.h>
#include <CoreFoundation/CoreFoundation.h>
#include <pthread.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

typedef struct {
	MIDIClientRef client;
	MIDIPortRef port;
	MIDIEndpointRef endpoint;
} xt_output_state;

typedef struct {
	MIDISysexSendRequest request;
	pthread_mutex_t mutex;
	pthread_cond_t condition;
	int completed;
} xt_sync_sysex_request;

static char *xt_copy_string_property(MIDIEndpointRef endpoint, CFStringRef property) {
	CFStringRef value = NULL;
	if (MIDIObjectGetStringProperty(endpoint, property, &value) != noErr || value == NULL) {
		return NULL;
	}
	CFIndex capacity = CFStringGetMaximumSizeForEncoding(CFStringGetLength(value), kCFStringEncodingUTF8) + 1;
	char *buffer = (char *)calloc((size_t)capacity, 1);
	if (buffer == NULL || !CFStringGetCString(value, buffer, capacity, kCFStringEncodingUTF8)) {
		free(buffer);
		buffer = NULL;
	}
	CFRelease(value);
	return buffer;
}

static int xt_destination_count(void) {
	return (int)MIDIGetNumberOfDestinations();
}

static MIDIClientRef xt_create_enumeration_client(OSStatus *result) {
	MIDIClientRef client = 0;
	*result = MIDIClientCreate(CFSTR("xtouch-cli enumeration"), NULL, NULL, &client);
	return client;
}

static void xt_dispose_client(MIDIClientRef client) {
	if (client != 0) {
		MIDIClientDispose(client);
	}
}

static MIDIEndpointRef xt_destination_at(int index) {
	return MIDIGetDestination((ItemCount)index);
}

static char *xt_destination_name(int index) {
	MIDIEndpointRef endpoint = xt_destination_at(index);
	char *name = xt_copy_string_property(endpoint, kMIDIPropertyDisplayName);
	if (name == NULL) {
		name = xt_copy_string_property(endpoint, kMIDIPropertyName);
	}
	return name;
}

static char *xt_destination_manufacturer(int index) {
	return xt_copy_string_property(xt_destination_at(index), kMIDIPropertyManufacturer);
}

static char *xt_destination_model(int index) {
	return xt_copy_string_property(xt_destination_at(index), kMIDIPropertyModel);
}

static int xt_destination_offline(int index) {
	SInt32 offline = 0;
	if (MIDIObjectGetIntegerProperty(xt_destination_at(index), kMIDIPropertyOffline, &offline) != noErr) {
		return 0;
	}
	return offline != 0;
}

static SInt32 xt_destination_unique_id(int index) {
	SInt32 unique_id = 0;
	MIDIObjectGetIntegerProperty(xt_destination_at(index), kMIDIPropertyUniqueID, &unique_id);
	return unique_id;
}

static SInt32 xt_destination_max_sysex_rate(int index) {
	SInt32 rate = 3125;
	MIDIObjectGetIntegerProperty(xt_destination_at(index), kMIDIPropertyMaxSysExSpeed, &rate);
	return rate;
}

static xt_output_state *xt_open_output(SInt32 unique_id, OSStatus *result) {
	xt_output_state *state = (xt_output_state *)calloc(1, sizeof(xt_output_state));
	if (state == NULL) {
		*result = -108;
		return NULL;
	}
	MIDIObjectRef object = 0;
	MIDIObjectType object_type = kMIDIObjectType_Other;
	*result = MIDIObjectFindByUniqueID(unique_id, &object, &object_type);
	if (*result != noErr || object == 0) {
		free(state);
		return NULL;
	}
	if (object_type != kMIDIObjectType_Destination && object_type != kMIDIObjectType_ExternalDestination) {
		free(state);
		*result = kMIDIWrongEndpointType;
		return NULL;
	}
	state->endpoint = (MIDIEndpointRef)object;

	*result = MIDIClientCreate(CFSTR("xtouch-cli"), NULL, NULL, &state->client);
	if (*result != noErr) {
		free(state);
		return NULL;
	}
	*result = MIDIOutputPortCreate(state->client, CFSTR("xtouch-cli output"), &state->port);
	if (*result != noErr) {
		MIDIClientDispose(state->client);
		free(state);
		return NULL;
	}
	return state;
}

static void xt_sysex_completed(MIDISysexSendRequest *request) {
	xt_sync_sysex_request *sync = (xt_sync_sysex_request *)request->completionRefCon;
	pthread_mutex_lock(&sync->mutex);
	sync->completed = 1;
	pthread_cond_signal(&sync->condition);
	pthread_mutex_unlock(&sync->mutex);
}

static OSStatus xt_send(xt_output_state *state, const uint8_t *bytes, uint32_t length) {
	if (state == NULL || bytes == NULL || length == 0) {
		return -50;
	}
	uint8_t *copy = (uint8_t *)malloc(length);
	if (copy == NULL) {
		return -108;
	}
	memcpy(copy, bytes, length);

	xt_sync_sysex_request sync;
	memset(&sync, 0, sizeof(sync));
	pthread_mutex_init(&sync.mutex, NULL);
	pthread_cond_init(&sync.condition, NULL);
	sync.request.destination = state->endpoint;
	sync.request.data = copy;
	sync.request.bytesToSend = length;
	sync.request.complete = false;
	sync.request.completionProc = xt_sysex_completed;
	sync.request.completionRefCon = &sync;

	OSStatus result = MIDISendSysex(&sync.request);
	if (result == noErr) {
		pthread_mutex_lock(&sync.mutex);
		while (!sync.completed) {
			pthread_cond_wait(&sync.condition, &sync.mutex);
		}
		pthread_mutex_unlock(&sync.mutex);
		if (sync.request.bytesToSend != 0) {
			result = kMIDIMessageSendErr;
		}
	}

	pthread_cond_destroy(&sync.condition);
	pthread_mutex_destroy(&sync.mutex);
	free(copy);
	return result;
}

static void xt_close_output(xt_output_state *state) {
	if (state == NULL) {
		return;
	}
	if (state->port != 0) {
		MIDIPortDispose(state->port);
	}
	if (state->client != 0) {
		MIDIClientDispose(state->client);
	}
	free(state);
}
*/
import "C"

import (
	"fmt"
	"runtime"
	"unsafe"
)

type coreMIDIOutput struct {
	state *C.xt_output_state
}

func Destinations() ([]Destination, error) {
	var status C.OSStatus
	client := C.xt_create_enumeration_client(&status)
	if client == 0 || status != 0 {
		return nil, fmt.Errorf("initialize CoreMIDI: OSStatus %d", int32(status))
	}
	defer C.xt_dispose_client(client)

	count := int(C.xt_destination_count())
	destinations := make([]Destination, 0, count)
	for index := 0; index < count; index++ {
		destinations = append(destinations, Destination{
			Index:        index,
			UniqueID:     int32(C.xt_destination_unique_id(C.int(index))),
			Name:         goString(C.xt_destination_name(C.int(index))),
			Manufacturer: goString(C.xt_destination_manufacturer(C.int(index))),
			Model:        goString(C.xt_destination_model(C.int(index))),
			Offline:      C.xt_destination_offline(C.int(index)) != 0,
			MaxSysExRate: int32(C.xt_destination_max_sysex_rate(C.int(index))),
		})
	}
	return destinations, nil
}

func OpenOutput(destination Destination) (Output, error) {
	if destination.UniqueID == 0 {
		return nil, fmt.Errorf("CoreMIDI destination %q has no stable unique ID", destination.Name)
	}
	var status C.OSStatus
	state := C.xt_open_output(C.SInt32(destination.UniqueID), &status)
	if state == nil || status != 0 {
		return nil, fmt.Errorf("open CoreMIDI destination %q (ID %d): OSStatus %d", destination.Name, destination.UniqueID, int32(status))
	}
	return &coreMIDIOutput{state: state}, nil
}

func (output *coreMIDIOutput) Send(message []byte) error {
	if output.state == nil {
		return fmt.Errorf("CoreMIDI output is closed")
	}
	if len(message) == 0 || uint64(len(message)) > uint64(^uint32(0)) {
		return fmt.Errorf("invalid MIDI message length %d", len(message))
	}
	status := C.xt_send(
		output.state,
		(*C.uint8_t)(unsafe.Pointer(&message[0])),
		C.uint32_t(len(message)),
	)
	runtime.KeepAlive(message)
	if status != 0 {
		return fmt.Errorf("send MIDI message: OSStatus %d", int32(status))
	}
	return nil
}

func (output *coreMIDIOutput) Close() error {
	if output.state != nil {
		C.xt_close_output(output.state)
		output.state = nil
	}
	return nil
}

func goString(value *C.char) string {
	if value == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(value))
	return C.GoString(value)
}
