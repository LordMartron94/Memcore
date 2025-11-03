package memcore

import (
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"time"
	"unsafe"
)

// ANSI color codes
const (
	colorReset   = "\033[0m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
	colorWhite   = "\033[37m"
	colorGray    = "\033[90m"

	colorBoldRed     = "\033[1;31m"
	colorBoldGreen   = "\033[1;32m"
	colorBoldYellow  = "\033[1;33m"
	colorBoldBlue    = "\033[1;34m"
	colorBoldMagenta = "\033[1;35m"
	colorBoldCyan    = "\033[1;36m"
	colorBoldWhite   = "\033[1;37m"
)

var internalPrefixes = []string{
	"runtime.", "reflect.", "memcore.", "testing.",
}

var callStackDepth uint = 3

// MemcoreInternalFunctionPrefixAdd adds a prefix to skip when determining which
// function created the pointer.
func MemcoreInternalFunctionPrefixAdd(prefix string) {
	internalPrefixes = append(internalPrefixes, prefix)
}

// MemcoreCallStackDepthSet sets the amount of calls to show when debugging pointer traces.
func MemcoreCallStackDepthSet(v uint) {
	callStackDepth = v
}

var enablePtrDebug bool = false

func MemcorePointersEnableDebug() {
	enablePtrDebug = true
}

var addressSpaceID uint32 = 0

type addressSpaceMapping map[uint32]addressSpaceMetadata

type addressSpaceMetadata struct {
	destroyed   bool
	baseAddress uintptr
}

var addressSpaceMap addressSpaceMapping = make(addressSpaceMapping)

type addressSpacePointersMap map[uint32]map[Pointer]struct{}

var addressSpacePointers addressSpacePointersMap = make(addressSpacePointersMap)

func MemcoreAddressSpaceRegister(baseAddr uintptr) uint32 {
	addressSpaceMap[addressSpaceID] = addressSpaceMetadata{
		baseAddress: baseAddr,
		destroyed:   false,
	}
	addressSpaceID++
	return addressSpaceID - 1
}

type Pointer struct {
	offset       uintptr
	addressSpace uint32
}

func (p Pointer) String() string {
	md, ok := pointerMetadataTable[p]
	if ok {
		statusStr := "inactive"
		if md.activePtr {
			statusStr = "active"
		}
		return fmt.Sprintf("Pointer{AS=%d, offset=0x%x, type=%s, status=%s}",
			p.addressSpace, p.offset, md.pointerType.Readable(), statusStr)
	}
	return fmt.Sprintf("Pointer{AS=%d, offset=0x%x, type=unknown, status=unknown}", p.addressSpace, p.offset)
}

func (p Pointer) IsValid() bool {
	if _, asOK := addressSpaceMap[p.addressSpace]; !asOK {
		return false
	}
	_, registered := pointerMetadataTable[p]
	return registered
}

func (p Pointer) CallStack() string {
	md := pointerMetadataTable[p]
	return md.creator
}

func PointerAddressSpace(pointer Pointer) uint32 {
	return pointer.addressSpace
}

func PointerOffset(pointer Pointer) uintptr {
	return pointer.offset
}

type pointerMetadata struct {
	timestamp   int64
	pointerType Type
	activePtr   bool
	creator     string
}

func isInternalFrame(f runtime.Frame) bool {
	for _, p := range internalPrefixes {
		if strings.Contains(f.Function, p) {
			return true
		}
	}
	return false
}

func MemcorePointerCreate(addressSpace uint32, offset uintptr, pointerType reflect.Type) Pointer {
	var pcs [16]uintptr
	n := runtime.Callers(2, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])

	var relevantFrames []runtime.Frame
	for {
		frame, more := frames.Next()
		if !isInternalFrame(frame) {
			relevantFrames = append(relevantFrames, frame)
		}
		if !more {
			break
		}
	}

	if len(relevantFrames) > int(callStackDepth) {
		relevantFrames = relevantFrames[len(relevantFrames)-int(callStackDepth):]
	}

	for i, j := 0, len(relevantFrames)-1; i < j; i, j = i+1, j-1 {
		relevantFrames[i], relevantFrames[j] = relevantFrames[j], relevantFrames[i]
	}

	var chain []string
	for _, f := range relevantFrames {
		chain = append(chain, filepath.Base(f.Function))
	}
	creatorChain := strings.Join(chain, " → ")

	last := relevantFrames[len(relevantFrames)-1]

	ptr := Pointer{
		addressSpace: addressSpace,
		offset:       offset,
	}

	md := pointerMetadata{
		pointerType: retrieveDataTypeID(pointerType),
		timestamp:   time.Now().UnixNano(),
		activePtr:   false,
		creator:     fmt.Sprintf("%s:%d (%s)", last.File, last.Line, creatorChain),
	}

	pointerMetadataTable[ptr] = md
	return ptr
}

func memcorePointerResolve(pointer Pointer) unsafe.Pointer {
	addressSpaceMd, exists := addressSpaceMap[pointer.addressSpace]

	if !exists {
		panic(fmt.Errorf("%sPointer Resolution Failed%s\n"+
			"  Reason: Address space %d does not exist\n"+
			"  Pointer: %s\n"+
			"  Hint: The address space may never have been registered",
			colorBoldRed, colorReset, pointer.addressSpace, pointer))
	}

	if addressSpaceMd.destroyed {
		panic(fmt.Errorf("%sPointer Resolution Failed%s\n"+
			"  Reason: Address space %d has been destroyed\n"+
			"  Pointer: %s\n"+
			"  Base Address: 0x%x\n"+
			"  Hint: Cannot dereference pointers from destroyed address spaces",
			colorBoldRed, colorReset, pointer.addressSpace, pointer, addressSpaceMd.baseAddress))
	}

	return unsafe.Add(unsafe.Pointer(addressSpaceMd.baseAddress), pointer.offset)
}

type Type uint16
type dataTypeToDataTypeIDMap map[reflect.Type]Type

var pointerMetadataTable = make(map[Pointer]pointerMetadata)

var dataTypeMap dataTypeToDataTypeIDMap = make(dataTypeToDataTypeIDMap)

func (t Type) Readable() string {
	for reflectType, customType := range dataTypeMap {
		if customType == t {
			name := reflectType.Name()
			if name == "" {
				return reflectType.String()
			}
			return name
		}
	}
	return "unknown"
}

var globalDataTypeID Type = 0

func retrieveDataTypeID(dataType reflect.Type) Type {
	if v, exist := dataTypeMap[dataType]; exist {
		return v
	}
	dataTypeMap[dataType] = globalDataTypeID
	globalDataTypeID++
	return globalDataTypeID - 1
}

func MemcorePointerRegister(pointer Pointer) {
	if _, ok := addressSpacePointers[pointer.addressSpace]; !ok {
		addressSpacePointers[pointer.addressSpace] = make(map[Pointer]struct{})
	}
	addressSpacePointers[pointer.addressSpace][pointer] = struct{}{}
	md := pointerMetadataTable[pointer]
	md.activePtr = true
	pointerMetadataTable[pointer] = md
}

func MemcorePointerUnregister(pointer Pointer) {
	md := pointerMetadataTable[pointer]
	md.activePtr = false
	pointerMetadataTable[pointer] = md

	delete(addressSpacePointers[pointer.addressSpace], pointer)
}

func MemcoreAddressSpaceUnregister(addressSpaceID uint32) {
	if nsSet, ok := addressSpacePointers[addressSpaceID]; ok {
		for ptr := range nsSet {
			MemcorePointerUnregister(ptr)
		}
	}

	md := addressSpaceMap[addressSpaceID]
	md.destroyed = true
	addressSpaceMap[addressSpaceID] = md
}

func MemcoreAddressSpaceClearPointers(addressSpaceID uint32) {
	if nsSet, ok := addressSpacePointers[addressSpaceID]; ok {
		for ptr := range nsSet {
			delete(pointerMetadataTable, ptr)
		}
		addressSpacePointers[addressSpaceID] = make(map[Pointer]struct{})
	}
}

func MemcorePointerBaseAddressUpdate(addressSpaceID uint32, newAddr uintptr) {
	addressSpaceMap[addressSpaceID] = addressSpaceMetadata{
		destroyed:   false,
		baseAddress: newAddr,
	}
}

func MemcorePointerDereferenceRaw(pointer Pointer) unsafe.Pointer {
	md, exists := pointerMetadataTable[pointer]

	if !exists {
		raisePtrError(fmt.Errorf("%sPointer Dereference Failed%s\n"+
			"  Reason: Pointer is not registered in metadata table\n"+
			"  Pointer: %s\n"+
			"  Hint: The pointer may have been created incorrectly or never registered",
			colorBoldRed, colorReset, pointer))
		return nil
	}

	if !md.activePtr {
		raisePtrError(fmt.Errorf("%sPointer Dereference Failed%s\n"+
			"  Reason: Pointer is inactive (not registered or has been unregistered)\n"+
			"  Pointer: %s\n"+
			"  Type: %s\n"+
			"  Created: %s\n"+
			"  Hint: Call MemcorePointerRegister() before dereferencing, or check if it was unregistered",
			colorBoldRed, colorReset, pointer, md.pointerType.Readable(), md.creator))
		return nil
	}

	return memcorePointerResolve(pointer)
}

func MemcorePointerDereferenceObject[T any](pointer Pointer) *T {
	ptr := MemcorePointerDereferenceRaw(pointer)
	md := pointerMetadataTable[pointer]
	expectedType := retrieveDataTypeID(reflect.TypeFor[T]())

	if md.pointerType != expectedType {
		panic(fmt.Errorf("%sType Mismatch Error%s\n"+
			"  Expected Type: %s (ID: %d)\n"+
			"  Actual Type: %s (ID: %d)\n"+
			"  Pointer: %s\n"+
			"  Hint: Use MemcorePointerDereferenceObjectUnsafe() to bypass type checking, or fix the type parameter",
			colorBoldRed, colorReset,
			reflect.TypeFor[T]().Name(), expectedType,
			md.pointerType.Readable(), md.pointerType,
			pointer))
	}

	return (*T)(ptr)
}

func MemcorePointerDereferenceObjectUnsafe[T any](pointer Pointer) *T {
	ptr := MemcorePointerDereferenceRaw(pointer)
	return (*T)(ptr)
}

func MemcorePointerUpdateType(pointer Pointer, newType reflect.Type) {
	md, exists := pointerMetadataTable[pointer]

	if !exists {
		raisePtrError(fmt.Errorf("%sPointer Type Update Failed%s\n"+
			"  Reason: Pointer is not registered in metadata table\n"+
			"  Pointer: %s\n"+
			"  New Type: %s\n"+
			"  Hint: Cannot update type of unregistered pointer",
			colorBoldRed, colorReset, pointer, newType.Name()))
		return
	}

	if !md.activePtr {
		raisePtrError(fmt.Errorf("%sPointer Type Update Failed%s\n"+
			"  Reason: Pointer is inactive\n"+
			"  Pointer: %s\n"+
			"  Old Type: %s\n"+
			"  New Type: %s\n"+
			"  Hint: Register the pointer before updating its type",
			colorBoldRed, colorReset, pointer, md.pointerType.Readable(), newType.Name()))
		return
	}

	oldType := md.pointerType.Readable()
	md.pointerType = retrieveDataTypeID(newType)
	pointerMetadataTable[pointer] = md

	if enablePtrDebug {
		fmt.Printf("%sPointer Type Updated%s: %s → %s (Pointer: %s)\n",
			colorBoldGreen, colorReset, oldType, newType.Name(), pointer)
	}
}

func printExistingPointers() {
	const boxWidth = 100

	// Header
	fmt.Printf("\n%s╔%s╗%s\n", colorBoldCyan, strings.Repeat("═", boxWidth-2), colorReset)
	headerText := "POINTER DEBUG REPORT"
	padding := (boxWidth - 2 - len(headerText)) / 2
	fmt.Printf("%s║%s%s%s%s║%s\n",
		colorBoldCyan,
		strings.Repeat(" ", padding),
		headerText,
		strings.Repeat(" ", boxWidth-2-padding-len(headerText)),
		colorBoldCyan,
		colorReset)
	fmt.Printf("%s╚%s╝%s\n\n", colorBoldCyan, strings.Repeat("═", boxWidth-2), colorReset)

	// Gather pointers
	keys := make([]Pointer, 0, len(pointerMetadataTable))
	for k := range pointerMetadataTable {
		keys = append(keys, k)
	}

	// Sort by timestamp
	slices.SortFunc(keys, func(p1, p2 Pointer) int {
		md1 := pointerMetadataTable[p1]
		md2 := pointerMetadataTable[p2]
		if md1.timestamp < md2.timestamp {
			return -1
		}
		if md1.timestamp > md2.timestamp {
			return 1
		}
		return 0
	})

	// Group by address space
	byAddressSpace := make(map[uint32][]Pointer)
	for _, ptr := range keys {
		as := ptr.addressSpace
		byAddressSpace[as] = append(byAddressSpace[as], ptr)
	}

	// Print statistics
	activeCount := 0
	inactiveCount := 0
	for _, md := range pointerMetadataTable {
		if md.activePtr {
			activeCount++
		} else {
			inactiveCount++
		}
	}

	fmt.Printf("%s┌─ Statistics %s\n", colorBoldWhite, strings.Repeat("─", boxWidth-14))
	fmt.Printf("%s│%s  %sTotal Pointers:%s %s%d%s  ", colorBoldWhite, colorReset, colorWhite, colorReset, colorBoldGreen, len(keys), colorReset)
	fmt.Printf("%sActive:%s %s%d%s  ", colorWhite, colorReset, colorGreen, activeCount, colorReset)
	fmt.Printf("%sInactive:%s %s%d%s\n", colorWhite, colorReset, colorRed, inactiveCount, colorReset)
	fmt.Printf("%s│%s  %sAddress Spaces:%s %s%d%s", colorBoldWhite, colorReset, colorWhite, colorReset, colorCyan, len(byAddressSpace), colorReset)

	// Count destroyed address spaces
	destroyedCount := 0
	for _, md := range addressSpaceMap {
		if md.destroyed {
			destroyedCount++
		}
	}
	if destroyedCount > 0 {
		fmt.Printf("  %sDestroyed:%s %s%d%s", colorWhite, colorReset, colorRed, destroyedCount, colorReset)
	}
	fmt.Printf("\n%s└%s\n\n", colorBoldWhite, strings.Repeat("─", boxWidth-1))

	// Print each address space
	asKeys := make([]uint32, 0, len(byAddressSpace))
	for as := range byAddressSpace {
		asKeys = append(asKeys, as)
	}
	slices.Sort(asKeys)

	for asIdx, as := range asKeys {
		pointers := byAddressSpace[as]
		asMd := addressSpaceMap[as]

		// Address space header
		headerLine := fmt.Sprintf("Address Space %d [Base: 0x%x]", as, asMd.baseAddress)
		if asMd.destroyed {
			headerLine += " [DESTROYED]"
		}
		headerLine += fmt.Sprintf(" (%d pointer%s)", len(pointers), pluralize(len(pointers)))

		remainingWidth := boxWidth - len(headerLine) - 4
		if remainingWidth < 0 {
			remainingWidth = 0
		}

		fmt.Printf("%s┌─ %s%s %s%s\n",
			colorBoldYellow,
			headerLine,
			colorYellow,
			strings.Repeat("─", remainingWidth),
			colorReset)

		for i, ptr := range pointers {
			md := pointerMetadataTable[ptr]

			// Status indicator
			statusColor := colorGreen
			statusSymbol := "●"
			statusText := "ACTIVE  "
			if !md.activePtr {
				statusColor = colorRed
				statusSymbol = "○"
				statusText = "INACTIVE"
			}

			// Index and status
			fmt.Printf("%s│%s  %s%03d%s) %s%s %s%s ",
				colorYellow, colorReset,
				colorGray, i, colorReset,
				statusColor, statusSymbol, statusText, colorReset)

			// Type and offset
			typeName := truncateString(md.pointerType.Readable(), 30)
			fmt.Printf("%sType:%s %s%-30s%s ",
				colorWhite, colorReset,
				colorMagenta, typeName, colorReset)

			fmt.Printf("%sOffset:%s %s0x%08x%s\n",
				colorWhite, colorReset,
				colorBlue, ptr.offset, colorReset)

			// Creator info (shortened path)
			creatorParts := strings.SplitN(md.creator, " ", 2)
			fileLine := creatorParts[0]
			function := ""
			if len(creatorParts) > 1 {
				function = strings.Trim(creatorParts[1], "()")
			}

			// Shorten file path
			if strings.Contains(fileLine, "/") {
				parts := strings.Split(fileLine, "/")
				if len(parts) > 3 {
					fileLine = ".../" + strings.Join(parts[len(parts)-3:], "/")
				}
			}

			fmt.Printf("%s│%s     %s└─%s %sLocation:%s %s%s%s",
				colorYellow, colorReset,
				colorGray, colorReset,
				colorWhite, colorReset,
				colorCyan, fileLine, colorReset)

			if function != "" {
				shortFunc := filepath.Base(function)
				fmt.Printf(" %s[%s]%s", colorGray, shortFunc, colorReset)
			}
			fmt.Printf("\n")

			// Timestamp
			t := time.Unix(0, md.timestamp)
			age := time.Since(t)
			ageStr := formatDuration(age)
			fmt.Printf("%s│%s        %sCreated:%s %s%s%s %s(%s ago)%s\n",
				colorYellow, colorReset,
				colorWhite, colorReset,
				colorGray, t.Format("2006-01-02 15:04:05.000"), colorReset,
				colorGray, ageStr, colorReset)

			// Separator between pointers
			if i < len(pointers)-1 {
				fmt.Printf("%s│%s\n", colorYellow, colorReset)
			}
		}

		fmt.Printf("%s└%s%s\n", colorYellow, strings.Repeat("─", boxWidth-1), colorReset)

		// Add spacing between address spaces
		if asIdx < len(asKeys)-1 {
			fmt.Println()
		}
	}

	fmt.Println()
}

func pluralize(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
	return fmt.Sprintf("%.1fd", d.Hours()/24)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func raisePtrError(errorMsg error) {
	if enablePtrDebug {
		printExistingPointers()
	}

	panic(errorMsg)
}

func MemcorePrintPointerDebug() {
	if enablePtrDebug {
		printExistingPointers()
	}
}
