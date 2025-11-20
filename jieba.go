package gojieba

/*
#cgo CXXFLAGS: -I./deps/cppjieba/include -I./deps/cppjieba/deps/limonp/include -DLOGGING_LEVEL=LL_WARNING -O3 -Wno-deprecated -Wno-unused-variable -std=c++11
#include <stdlib.h>
#include "jieba.h"
*/
import "C"

import (
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"unsafe"
)

type TokenizeMode int

const (
	DefaultMode TokenizeMode = iota
	SearchMode
)

type Word struct {
	Str   string
	Start int
	End   int
}

type Jieba struct {
	jieba C.Jieba
	freed int32
}

func NewJieba(paths ...string) *Jieba {
	dictpaths := getDictPaths(paths...)

	for _, path := range dictpaths {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			panic(fmt.Sprintf("Dictionary file does not exist: %s", path))
		}
	}

	dpath, hpath, upath, ipath, spath := C.CString(dictpaths[0]), C.CString(dictpaths[1]), C.CString(dictpaths[2]), C.CString(dictpaths[3]), C.CString(dictpaths[4])
	defer C.free(unsafe.Pointer(dpath))
	defer C.free(unsafe.Pointer(hpath))
	defer C.free(unsafe.Pointer(upath))
	defer C.free(unsafe.Pointer(ipath))
	defer C.free(unsafe.Pointer(spath))
	jieba := &Jieba{
		C.NewJieba(
			dpath,
			hpath,
			upath,
			ipath,
			spath,
		),
		0,
	}
	runtime.SetFinalizer(jieba, (*Jieba).Free)
	return jieba
}

func (x *Jieba) Free() {
	if atomic.CompareAndSwapInt32(&x.freed, 0, 1) {
		C.FreeJieba(x.jieba)
	}
}

func (x *Jieba) FreeWithTrim() {
	x.Free()
	C.Trim()
}

func (x *Jieba) WithTrim() *Jieba {
	runtime.SetFinalizer(x, nil)
	runtime.SetFinalizer(x, (*Jieba).FreeWithTrim)
	return x
}

// --- 核心优化部分: Cut 使用 offsets 实现零拷贝 ---

func (x *Jieba) Cut(s string, hmm bool) []string {
	c_int_hmm := C.int(0)
	if hmm {
		c_int_hmm = 1
	}
	cstr := C.CString(s)
	defer C.free(unsafe.Pointer(cstr))

	// 调用返回 Word* 的新接口
	var words *C.Word = C.Cut(x.jieba, cstr, c_int_hmm)

	// Word* 是一个连续的内存块，直接 free 即可，不需要 FreeWords
	defer C.free(unsafe.Pointer(words))

	return convertCWordToSlice(s, words)
}

// CutForSearch 也使用了优化后的接口
func (x *Jieba) CutForSearch(s string, hmm bool) []string {
	c_int_hmm := C.int(0)
	if hmm {
		c_int_hmm = 1
	}
	cstr := C.CString(s)
	defer C.free(unsafe.Pointer(cstr))

	var words *C.Word = C.CutForSearch(x.jieba, cstr, c_int_hmm)
	defer C.free(unsafe.Pointer(words))

	return convertCWordToSlice(s, words)
}

// --- 保持原样部分: CutAll 返回 char** ---

func (x *Jieba) CutAll(s string) []string {
	cstr := C.CString(s)
	defer C.free(unsafe.Pointer(cstr))

	// 旧接口返回 char**
	var words **C.char = C.CutAll(x.jieba, cstr)

	// 必须使用 C.FreeWords 来循环释放 char*
	defer C.FreeWords(words)

	return cstrings(words)
}

// --- 保持原样部分: Tag 返回 char** ---

func (x *Jieba) Tag(s string) []string {
	cstr := C.CString(s)
	defer C.free(unsafe.Pointer(cstr))

	var words **C.char = C.Tag(x.jieba, cstr)
	defer C.FreeWords(words)

	return cstrings(words)
}

func (x *Jieba) AddWord(s string) {
	cstr := C.CString(s)
	defer C.free(unsafe.Pointer(cstr))
	C.AddWord(x.jieba, cstr)
}

func (x *Jieba) AddWordEx(s string, freq int, tag string) {
	cstr := C.CString(s)
	ctag := C.CString(tag)
	defer C.free(unsafe.Pointer(ctag))
	defer C.free(unsafe.Pointer(cstr))
	C.AddWordEx(x.jieba, cstr, C.int(freq), ctag)
}

func (x *Jieba) RemoveWord(s string) {
	cstr := C.CString(s)
	defer C.free(unsafe.Pointer(cstr))
	C.RemoveWord(x.jieba, cstr)
}

// Tokenize 接口，返回详细的结构体
func (x *Jieba) Tokenize(s string, mode TokenizeMode, hmm bool) []Word {
	c_int_hmm := C.int(0)
	if hmm {
		c_int_hmm = 1
	}
	cstr := C.CString(s)
	defer C.free(unsafe.Pointer(cstr))

	var words *C.Word = C.Tokenize(x.jieba, cstr, C.TokenizeMode(mode), c_int_hmm)
	defer C.free(unsafe.Pointer(words))

	return convertCWordToStructs(s, words)
}

type WordWeight struct {
	Word   string
	Weight float64
}

func (x *Jieba) Extract(s string, topk int) []string {
	cstr := C.CString(s)
	defer C.free(unsafe.Pointer(cstr))

	var words **C.char = C.Extract(x.jieba, cstr, C.int(topk))
	defer C.FreeWords(words)

	return cstrings(words)
}

func (x *Jieba) ExtractWithWeight(s string, topk int) []WordWeight {
	cstr := C.CString(s)
	defer C.free(unsafe.Pointer(cstr))

	words := C.ExtractWithWeight(x.jieba, cstr, C.int(topk))
	defer C.FreeWordWeights(words)

	return cwordweights(words)
}

// --- 辅助转换函数 ---

// [新] 将 *C.Word (offsets) 转为 []string (零拷贝)
func convertCWordToSlice(s string, x *C.Word) []string {
	var res []string
	p := x
	// 哨兵检测：假设 C++ 返回以 {0,0} 结尾
	// 或者你也可以在 C 结构体里加个 count，但这里沿用哨兵模式
	for p != nil && p.len != 0 {
		start := int(p.offset)
		end := start + int(p.len)
		if start <= end && end <= len(s) {
			res = append(res, s[start:end])
		}
		// 指针移动到下一个 struct
		p = (*C.Word)(unsafe.Pointer(uintptr(unsafe.Pointer(p)) + unsafe.Sizeof(*p)))
	}
	return res
}

// [新] 将 *C.Word 转为 []Word (Go Struct)
func convertCWordToStructs(s string, x *C.Word) []Word {
	var res []Word
	p := x
	for p != nil && p.len != 0 {
		start := int(p.offset)
		end := start + int(p.len)
		if start <= end && end <= len(s) {
			res = append(res, Word{
				Str:   s[start:end],
				Start: start,
				End:   end,
			})
		}
		p = (*C.Word)(unsafe.Pointer(uintptr(unsafe.Pointer(p)) + unsafe.Sizeof(*p)))
	}
	return res
}

// [旧] 将 CWordWeight 转为 []WordWeight
func cwordweights(x *C.struct_CWordWeight) []WordWeight {
	var s []WordWeight
	eltSize := unsafe.Sizeof(*x)
	for (*x).word != nil {
		ww := WordWeight{
			C.GoString(((C.struct_CWordWeight)(*x)).word),
			float64((*x).weight),
		}
		s = append(s, ww)
		x = (*C.struct_CWordWeight)(unsafe.Pointer(uintptr(unsafe.Pointer(x)) + eltSize))
	}
	return s
}
