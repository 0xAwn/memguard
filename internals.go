package memguard

import (
	"crypto/rand"
	"os"
	"sync"
	"unsafe"

	"github.com/awnumar/memguard/memcall"
)

var (
	// Ascertain and store the system memory page size.
	pageSize = os.Getpagesize()

	// Canary value that acts as an alarm in case of disallowed memory access.
	canary = createCanary()

	// Create a dedicated sync object for the CatchInterrupt function.
	catchInterruptOnce sync.Once

	// Array of all active containers, and associated mutex.
	enclaves      []*container
	enclavesMutex = &sync.Mutex{}
)

// Create and allocate a canary value. Return to caller.
func createCanary() []byte {
	// Canary length rounded to page size.
	roundedLen := roundToPageSize(32)

	// Therefore the total length is...
	totalLen := (2 * pageSize) + roundedLen

	// Allocate it.
	memory, err := memcall.Alloc(totalLen)
	if err != nil {
		SafePanic(err)
	}

	// Make the guard pages inaccessible.
	if err := memcall.Protect(memory[:pageSize], false, false); err != nil {
		SafePanic(err)
	}
	if err := memcall.Protect(memory[pageSize+roundedLen:], false, false); err != nil {
		SafePanic(err)
	}

	// Lock the pages that will hold the canary.
	if err := memcall.Lock(memory[pageSize : pageSize+roundedLen]); err != nil {
		SafePanic(err)
	}

	// Fill the memory with cryptographically-secure random bytes (the canary value).
	c := getBytes(uintptr(unsafe.Pointer(&memory[pageSize+roundedLen-32])), 32)
	fillRandBytes(c)

	// Tell the kernel that the canary value should be immutable.
	if err := memcall.Protect(memory[pageSize:pageSize+roundedLen], true, false); err != nil {
		SafePanic(err)
	}

	// Return a slice that describes the correct portion of memory.
	return c
}

// Round a length to a multiple of the system page size.
func roundToPageSize(length int) int {
	return (length + (pageSize - 1)) & (^(pageSize - 1))
}

// Get a slice that describes all memory related to a Enclave.
func getAllMemory(b *container) []byte {
	// Calculate the size of the entire container's memory.
	roundedBufLen := roundToPageSize(len(b.buffer) + 32)

	// Calculate the address of the start of the memory.
	memAddr := uintptr(unsafe.Pointer(&b.buffer[0])) - uintptr((roundedBufLen-len(b.buffer))+pageSize)

	// Calculate the size of the entire memory.
	memLen := (pageSize * 2) + roundedBufLen

	// Use this information to generate a slice and return it.
	return getBytes(memAddr, memLen)
}

// Convert a pointer and length to a byte slice that describes that memory.
func getBytes(ptr uintptr, len int) []byte {
	var sl = struct {
		addr uintptr
		len  int
		cap  int
	}{ptr, len, len}
	return *(*[]byte)(unsafe.Pointer(&sl))
}

// Takes a byte slice and fills it with random data.
func fillRandBytes(b []byte) {
	// Read len(b) bytes into the buffer.
	if _, err := rand.Read(b); err != nil {
		panic("memguard.csprng(): could not get random bytes")
	}
}

// Wipes a byte slice with zeroes.
func wipeBytes(buf []byte) {
	for i := range buf {
		buf[i] = 0
	}
}
