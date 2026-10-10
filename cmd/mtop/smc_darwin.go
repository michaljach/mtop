//go:build darwin && cgo

package main

/*
#cgo LDFLAGS: -framework IOKit
#include <IOKit/IOKitLib.h>
#include <string.h>

typedef struct { char major, minor, build, reserved; UInt16 release; } smcVers;
typedef struct { UInt16 version, length; UInt32 cpuPLimit, gpuPLimit, memPLimit; } smcPLimit;
typedef struct { UInt32 dataSize, dataType; char dataAttributes; } smcKeyInfo;
typedef struct {
	UInt32 key;
	smcVers vers;
	smcPLimit pLimit;
	smcKeyInfo keyInfo;
	char result, status, data8;
	UInt32 data32;
	unsigned char bytes[32];
} smcParam;

enum { smcReadBytes = 5, smcReadIndex = 8, smcReadInfo = 9 };

static io_connect_t smcConn;

static int smcOpen(void) {
	io_service_t svc = IOServiceGetMatchingService(kIOMainPortDefault, IOServiceMatching("AppleSMC"));
	if (!svc) return -1;
	kern_return_t kr = IOServiceOpen(svc, mach_task_self(), 0, &smcConn);
	IOObjectRelease(svc);
	return kr == KERN_SUCCESS ? 0 : -1;
}

static int smcCall(smcParam *in, smcParam *out) {
	size_t size = sizeof(smcParam);
	memset(out, 0, sizeof(smcParam));
	return IOConnectCallStructMethod(smcConn, 2, in, sizeof(smcParam), out, &size) == KERN_SUCCESS && !out->result ? 0 : -1;
}

static UInt32 smcKeyAt(UInt32 i) {
	smcParam in = {0}, out;
	in.data8 = smcReadIndex;
	in.data32 = i;
	return smcCall(&in, &out) ? 0 : out.key;
}

// smcRead fills buf with the key's raw value and returns its 4-char type code, or 0 on failure.
static UInt32 smcRead(UInt32 key, unsigned char *buf, UInt32 *size) {
	smcParam in = {0}, out;
	in.key = key;
	in.data8 = smcReadInfo;
	if (smcCall(&in, &out)) return 0;
	smcKeyInfo info = out.keyInfo;
	in.keyInfo.dataSize = info.dataSize;
	in.data8 = smcReadBytes;
	if (smcCall(&in, &out)) return 0;
	*size = info.dataSize > 32 ? 32 : info.dataSize;
	memcpy(buf, out.bytes, *size);
	return info.dataType;
}
*/
import "C"

import (
	"encoding/binary"
	"math"
	"sync"
	"unsafe"
)

var smc struct {
	once     sync.Once
	cpu, gpu []C.UInt32 // float temperature keys
}

func fourCC(s string) C.UInt32 { return C.UInt32(binary.BigEndian.Uint32([]byte(s))) }

func smcFloat(key C.UInt32) (float64, bool) {
	var buf [32]byte
	var size C.UInt32
	typ := C.smcRead(key, (*C.uchar)(unsafe.Pointer(&buf[0])), &size)
	if typ != fourCC("flt ") || size != 4 {
		return 0, false
	}
	return float64(math.Float32frombits(binary.LittleEndian.Uint32(buf[:4]))), true
}

// smcInit enumerates the SMC once and keeps the Apple Silicon CPU (Tp*, Te*)
// and GPU (Tg*) temperature keys.
func smcInit() {
	if C.smcOpen() != 0 {
		return
	}
	var cnt [32]byte
	var size C.UInt32
	if C.smcRead(fourCC("#KEY"), (*C.uchar)(unsafe.Pointer(&cnt[0])), &size) == 0 {
		return
	}
	n := binary.BigEndian.Uint32(cnt[:4])
	for i := uint32(0); i < n; i++ {
		k := C.smcKeyAt(C.UInt32(i))
		var name [4]byte
		binary.BigEndian.PutUint32(name[:], uint32(k))
		if name[0] != 'T' {
			continue
		}
		if _, ok := smcFloat(k); !ok {
			continue
		}
		switch name[1] {
		case 'p', 'e':
			smc.cpu = append(smc.cpu, k)
		case 'g':
			smc.gpu = append(smc.gpu, k)
		}
	}
}

// avgTemp averages the sensors that report a plausible die temperature;
// unpopulated slots read as 0 or small noise.
func avgTemp(keys []C.UInt32) float64 {
	var sum float64
	var n int
	for _, k := range keys {
		if v, ok := smcFloat(k); ok && v > 15 && v < 130 {
			sum += v
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// smcTemps returns the average CPU and GPU temperatures, 0 when unknown.
func smcTemps() (cpu, gpu float64) {
	smc.once.Do(smcInit)
	return avgTemp(smc.cpu), avgTemp(smc.gpu)
}
