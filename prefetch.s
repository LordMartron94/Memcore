//go:build amd64

#include "textflag.h"

// void platformPrefetchReadAsm(void* ptr)
TEXT ·platformPrefetchReadAsm(SB), NOSPLIT, $0-8
    MOVQ ptr+0(FP), AX
    PREFETCHT0 (AX)
    RET
