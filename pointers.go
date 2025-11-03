package memcore

import (
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// ────────────────────────────────────────────────────────────────
//  ANSI COLORS
// ────────────────────────────────────────────────────────────────

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

// ────────────────────────────────────────────────────────────────
//  CORE TYPES
// ────────────────────────────────────────────────────────────────

type Pointer struct {
	offset       uintptr
	addressSpace uint32
}

type Type uint16

type pointerMetadata struct {
	timestamp   int64
	pointerType Type
	activePtr   bool
	creatorID   uint32
}

type creatorInfo struct {
	once        sync.Once
	resolvedStr string
	pcs         [16]uintptr
	n           int
}

// ────────────────────────────────────────────────────────────────
//  GLOBALS
// ────────────────────────────────────────────────────────────────

const shardCount = 64 // must be power of two

type pointerShard struct {
	table map[Pointer]pointerMetadata
}

var pointerShards [shardCount]*pointerShard

func init() {
	for i := range pointerShards {
		pointerShards[i] = &pointerShard{table: make(map[Pointer]pointerMetadata, 8192)}
	}
}

func shardIndex(p Pointer) int {
	return int((p.offset ^ uintptr(p.addressSpace)) & (shardCount - 1))
}

var (
	addressSpaceMap = make(map[uint32]addressSpaceMetadata)
	addressSpaceID  uint32
)

type addressSpaceMetadata struct {
	destroyed   bool
	baseAddress uintptr
	pointers    []Pointer
}

var (
	globalPointerClock uint64
	creatorTable       = make(map[uint32]*creatorInfo)
	creatorIDCounter   uint32
	dataTypeMap        = make(dataTypeToDataTypeIDMap)
	globalDataTypeID   Type
	internalPrefixes        = []string{"runtime.", "reflect.", "memcore.", "testing."}
	callStackDepth     uint = 3
	enablePtrDebug          = false
)

// ────────────────────────────────────────────────────────────────
//  RESET AND CONFIG
// ────────────────────────────────────────────────────────────────

func MemcoreResetState(resetFunctions bool) {
	for i := range pointerShards {
		pointerShards[i].table = make(map[Pointer]pointerMetadata, 8192)
	}
	addressSpaceMap = make(map[uint32]addressSpaceMetadata)
	addressSpaceID = 0
	globalPointerClock = 0
	creatorTable = make(map[uint32]*creatorInfo)
	creatorIDCounter = 0
	dataTypeMap = make(dataTypeToDataTypeIDMap)
	globalDataTypeID = 0
	if resetFunctions {
		functionRegistry = make(map[FunctionID]registeredFunction)
		functionIDCounter = 0
	}
}

func MemcoreInternalFunctionPrefixAdd(prefix string) {
	internalPrefixes = append(internalPrefixes, prefix)
}

func MemcoreCallStackDepthSet(v uint) {
	callStackDepth = v
}

func MemcorePointersEnableDebug() {
	enablePtrDebug = true
}

// ────────────────────────────────────────────────────────────────
//  ADDRESS SPACE
// ────────────────────────────────────────────────────────────────

func MemcoreAddressSpaceRegister(baseAddr uintptr) uint32 {
	id := atomic.AddUint32(&addressSpaceID, 1) - 1
	addressSpaceMap[id] = addressSpaceMetadata{
		baseAddress: baseAddr,
		destroyed:   false,
		pointers:    make([]Pointer, 0, 256),
	}
	return id
}

func MemcoreAddressSpaceUnregister(id uint32) {
	as, ok := addressSpaceMap[id]
	if !ok {
		return
	}
	as.destroyed = true
	addressSpaceMap[id] = as
}

func MemcoreAddressSpaceClearPointers(id uint32) {
	as, ok := addressSpaceMap[id]
	if !ok {
		return
	}
	as.pointers = nil
	addressSpaceMap[id] = as
}

func MemcorePointerBaseAddressUpdate(id uint32, newAddr uintptr) {
	as := addressSpaceMap[id]
	as.destroyed = false
	as.baseAddress = newAddr
	addressSpaceMap[id] = as
}

// ────────────────────────────────────────────────────────────────
//  POINTER CREATION AND REGISTRATION
// ────────────────────────────────────────────────────────────────

//go:nosplit
func memcorePointerCreateFast(addressSpace uint32, offset uintptr, typeID Type) Pointer {
	globalPointerClock++
	ptr := Pointer{addressSpace: addressSpace, offset: offset}
	shard := pointerShards[shardIndex(ptr)]
	shard.table[ptr] = pointerMetadata{
		pointerType: typeID,
		timestamp:   int64(globalPointerClock),
		activePtr:   false,
	}
	return ptr
}

func memcorePointerCreateDebug(addressSpace uint32, offset uintptr, t reflect.Type) Pointer {
	ptr := Pointer{addressSpace: addressSpace, offset: offset}

	var id uint32
	var info creatorInfo
	info.n = runtime.Callers(2, info.pcs[:])
	id = atomic.AddUint32(&creatorIDCounter, 1) - 1
	creatorTable[id] = &info

	globalPointerClock++
	shard := pointerShards[shardIndex(ptr)]
	shard.table[ptr] = pointerMetadata{
		pointerType: retrieveDataTypeID(t),
		timestamp:   int64(globalPointerClock),
		activePtr:   false,
		creatorID:   id,
	}
	return ptr
}

func MemcorePointerCreate(addressSpace uint32, offset uintptr, t reflect.Type) Pointer {
	if enablePtrDebug {
		return memcorePointerCreateDebug(addressSpace, offset, t)
	}
	return memcorePointerCreateFast(addressSpace, offset, retrieveDataTypeID(t))
}

func MemcorePointerRegister(p Pointer) {
	shard := pointerShards[shardIndex(p)]
	md, ok := shard.table[p]
	if !ok {
		return
	}
	md.activePtr = true
	shard.table[p] = md

	if enablePtrDebug {
		as := addressSpaceMap[p.addressSpace]
		as.pointers = append(as.pointers, p)
		addressSpaceMap[p.addressSpace] = as
	}
}

func MemcorePointerUnregister(p Pointer) {
	shard := pointerShards[shardIndex(p)]
	md, ok := shard.table[p]
	if !ok {
		return
	}
	md.activePtr = false
	shard.table[p] = md
}

// ────────────────────────────────────────────────────────────────
//  POINTER QUERY METHODS
// ────────────────────────────────────────────────────────────────

func (p Pointer) String() string {
	md, ok := pointerShards[shardIndex(p)].table[p]
	if ok {
		status := "inactive"
		if md.activePtr {
			status = "active"
		}
		return fmt.Sprintf("Pointer{AS=%d, offset=0x%x, type=%s, status=%s}",
			p.addressSpace, p.offset, md.pointerType.Readable(), status)
	}
	return fmt.Sprintf("Pointer{AS=%d, offset=0x%x, type=unknown, status=unknown}", p.addressSpace, p.offset)
}

func (p Pointer) IsValid() bool {
	as, ok := addressSpaceMap[p.addressSpace]
	if !ok || as.destroyed {
		return false
	}
	md, registered := pointerShards[shardIndex(p)].table[p]
	return registered && md.activePtr
}

func (p Pointer) BelongsToAddressSpace(target uint32) bool {
	return p.addressSpace == target
}

func (p Pointer) BelongsToAddressSpaceOfPointer(target Pointer) bool {
	return p.addressSpace == target.addressSpace
}

func (p Pointer) CallStack() string {
	md, ok := pointerShards[shardIndex(p)].table[p]
	if !ok {
		return "unknown"
	}
	return resolveCreatorString(md.creatorID)
}

func PointerAddressSpace(p Pointer) uint32 { return p.addressSpace }
func PointerOffset(p Pointer) uintptr      { return p.offset }

// ────────────────────────────────────────────────────────────────
//  POINTER DEREFERENCING
// ────────────────────────────────────────────────────────────────

func memcorePointerResolve(p Pointer) unsafe.Pointer {
	as := addressSpaceMap[p.addressSpace]
	if as.destroyed {
		panic(fmt.Errorf("address space %d destroyed", p.addressSpace))
	}
	return unsafe.Add(unsafe.Pointer(as.baseAddress), p.offset)
}

func MemcorePointerDereferenceRaw(p Pointer) unsafe.Pointer {
	shard := pointerShards[shardIndex(p)]
	md, ok := shard.table[p]
	if !ok || !md.activePtr {
		raisePtrError(fmt.Errorf("invalid or inactive pointer: %v", p))
	}
	return memcorePointerResolve(p)
}

func MemcorePointerDereferenceObject[T any](p Pointer) *T {
	ptr := MemcorePointerDereferenceRaw(p)
	md := pointerShards[shardIndex(p)].table[p]
	expected := retrieveDataTypeID(reflect.TypeFor[T]())
	if md.pointerType != expected {
		panic(fmt.Errorf("%stype mismatch:%s expected %s got %s",
			colorBoldRed, colorReset,
			reflect.TypeFor[T]().Name(), md.pointerType.Readable()))
	}
	return (*T)(ptr)
}

func MemcorePointerDereferenceObjectUnsafe[T any](p Pointer) *T {
	return (*T)(MemcorePointerDereferenceRaw(p))
}

// ────────────────────────────────────────────────────────────────
//  POINTER TYPE UPDATE
// ────────────────────────────────────────────────────────────────

func MemcorePointerUpdateType(p Pointer, newType reflect.Type) {
	shard := pointerShards[shardIndex(p)]
	md, ok := shard.table[p]
	if !ok {
		raisePtrError(fmt.Errorf("Pointer not registered: %v", p))
	}
	if !md.activePtr {
		raisePtrError(fmt.Errorf("Pointer inactive: %v", p))
	}
	oldType := md.pointerType.Readable()
	md.pointerType = retrieveDataTypeID(newType)
	shard.table[p] = md
	if enablePtrDebug {
		fmt.Printf("%sPointer type updated%s: %s → %s (%s)\n",
			colorBoldGreen, colorReset, oldType, newType.Name(), p)
	}
}

// ────────────────────────────────────────────────────────────────
//  CREATOR + TYPE REGISTRY
// ────────────────────────────────────────────────────────────────

func isInternalFrame(f runtime.Frame) bool {
	for _, p := range internalPrefixes {
		if strings.Contains(f.Function, p) {
			return true
		}
	}
	return false
}

func resolveCreatorString(id uint32) string {
	if id == 0 {
		return "unknown"
	}
	info, ok := creatorTable[id]
	if !ok {
		return "unknown"
	}
	info.once.Do(func() {
		frames := runtime.CallersFrames(info.pcs[:info.n])
		var chain []string
		for {
			f, more := frames.Next()
			if !isInternalFrame(f) {
				chain = append(chain, filepath.Base(f.Function))
			}
			if !more {
				break
			}
		}
		slices.Reverse(chain)
		info.resolvedStr = strings.Join(chain, " → ")
	})
	return info.resolvedStr
}

type dataTypeToDataTypeIDMap map[reflect.Type]Type

func (t Type) Readable() string {
	for rt, id := range dataTypeMap {
		if id == t {
			if name := rt.Name(); name != "" {
				return name
			}
			return rt.String()
		}
	}
	return "unknown"
}

func retrieveDataTypeID(rt reflect.Type) Type {
	if v, ok := dataTypeMap[rt]; ok {
		return v
	}
	id := globalDataTypeID
	dataTypeMap[rt] = id
	globalDataTypeID++
	return id
}

// ────────────────────────────────────────────────────────────────
//  FUNCTION REGISTRY
// ────────────────────────────────────────────────────────────────

type FunctionID uint64

type registeredFunction struct {
	id        FunctionID
	fn        reflect.Value
	fnType    reflect.Type
	createdAt time.Time
}

var (
	functionRegistry             = make(map[FunctionID]registeredFunction)
	functionIDCounter FunctionID = 0
)

func MemcoreFunctionRegister(fn any) FunctionID {
	v := reflect.ValueOf(fn)
	t := v.Type()
	if t.Kind() != reflect.Func {
		panic(fmt.Errorf("not a function: %s", t.Kind()))
	}
	id := functionIDCounter
	functionIDCounter++
	functionRegistry[id] = registeredFunction{
		id:        id,
		fn:        v,
		fnType:    t,
		createdAt: time.Now(),
	}
	return id
}

func MemcoreFunctionGet(id FunctionID) reflect.Value {
	fn, ok := functionRegistry[id]
	if !ok {
		panic(fmt.Errorf("function not found: %v", id))
	}
	return fn.fn
}

func MemcoreFunctionGetTyped[T any](id FunctionID) T {
	fn := MemcoreFunctionGet(id)
	return fn.Interface().(T)
}

func MemcoreFunctionRegisterTyped[T any](fn T) FunctionID {
	return MemcoreFunctionRegister(fn)
}

func MemcoreFunctionCall(id FunctionID, args ...any) []reflect.Value {
	fn := MemcoreFunctionGet(id)
	if len(args) != fn.Type().NumIn() {
		panic(fmt.Errorf("arg count mismatch: expected %d got %d", fn.Type().NumIn(), len(args)))
	}
	in := make([]reflect.Value, len(args))
	for i, a := range args {
		in[i] = reflect.ValueOf(a)
	}
	return fn.Call(in)
}

func MemcoreFunctionCallTyped[T any](id FunctionID, args ...any) T {
	out := MemcoreFunctionCall(id, args...)
	if len(out) == 0 {
		var zero T
		return zero
	}
	return out[0].Interface().(T)
}

// ────────────────────────────────────────────────────────────────
//  DEBUG + UTILITIES
// ────────────────────────────────────────────────────────────────

func raisePtrError(err error) {
	if enablePtrDebug {
		printExistingPointers()
	}
	panic(err)
}

func MemcorePrintPointerDebug() {
	if enablePtrDebug {
		printExistingPointers()
	}
}

func printExistingPointers() {
	const boxWidth = 100

	fmt.Printf("\n%s╔%s╗%s\n", colorBoldCyan, strings.Repeat("═", boxWidth-2), colorReset)
	header := "POINTER DEBUG REPORT"
	padding := (boxWidth - 2 - len(header)) / 2
	fmt.Printf("%s║%s%s%s%s║%s\n", colorBoldCyan, strings.Repeat(" ", padding),
		header, strings.Repeat(" ", boxWidth-2-padding-len(header)), colorBoldCyan, colorReset)
	fmt.Printf("%s╚%s╝%s\n\n", colorBoldCyan, strings.Repeat("═", boxWidth-2), colorReset)

	keys := make([]Pointer, 0, 2048)
	for _, shard := range pointerShards {
		for k := range shard.table {
			keys = append(keys, k)
		}
	}
	slices.SortFunc(keys, func(a, b Pointer) int {
		ma := pointerShards[shardIndex(a)].table[a]
		mb := pointerShards[shardIndex(b)].table[b]
		if ma.timestamp < mb.timestamp {
			return -1
		}
		if ma.timestamp > mb.timestamp {
			return 1
		}
		return 0
	})

	byAS := make(map[uint32][]Pointer)
	for _, p := range keys {
		byAS[p.addressSpace] = append(byAS[p.addressSpace], p)
	}

	active, inactive := 0, 0
	for _, shard := range pointerShards {
		for _, md := range shard.table {
			if md.activePtr {
				active++
			} else {
				inactive++
			}
		}
	}

	fmt.Printf("%s┌─ Stats %s\n", colorBoldWhite, strings.Repeat("─", boxWidth-10))
	fmt.Printf("%s│%s Total:%s %d  Active:%s %d%s  Inactive:%s %d%s\n",
		colorBoldWhite, colorReset, colorWhite, len(keys),
		colorGreen, active, colorReset, colorRed, inactive, colorReset)
	fmt.Printf("%s└%s\n\n", colorBoldWhite, strings.Repeat("─", boxWidth-1))

	for as, ps := range byAS {
		asMd := addressSpaceMap[as]
		status := ""
		if asMd.destroyed {
			status = " [DESTROYED]"
		}
		fmt.Printf("%sAddressSpace %d%s (%d ptr%s)%s\n",
			colorBoldYellow, as, status, len(ps), pluralize(len(ps)), colorReset)
		for i, p := range ps {
			md := pointerShards[shardIndex(p)].table[p]
			color := colorGreen
			if !md.activePtr {
				color = colorRed
			}
			fmt.Printf(" %s%03d)%s %-30s Offset 0x%x\n",
				color, i, colorReset, truncateString(md.pointerType.Readable(), 30), p.offset)
		}
		fmt.Println()
	}
}

func pluralize(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
