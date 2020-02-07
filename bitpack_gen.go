// +build ignore

package main

// This file is based on the code from https://github.com/kostya-sh/parquet-go
// Copyright (c) 2015 Konstantin Shaposhnikov

import (
	"bytes"
	"fmt"
	"go/format"
	"io"
	"io/ioutil"
	"log"
	"strings"
)

func genExpr(maxWidth int, bw int, i int, startBit int) (expr string, newStartBit int) {
	byteShift := 0
	firstCurByteBit := startBit - startBit%8
	for bw != 0 {
		curByte := startBit / 8
		bitsInCurByte := bw
		if bitsLeft := startBit - firstCurByteBit + 1; bitsInCurByte > bitsLeft {
			bitsInCurByte = bitsLeft
		}
		shiftSize := 7 - startBit%8
		mask := 1<<uint(bitsInCurByte) - 1

		if len(expr) != 0 {
			expr += " | "
		}
		expr += fmt.Sprintf("uint%d((data[%d] >> %d) & %d) << %d",
			maxWidth, curByte, shiftSize, mask, byteShift)

		bw -= bitsInCurByte
		startBit -= bitsInCurByte
		if startBit < firstCurByteBit {
			startBit = firstCurByteBit + 15
			firstCurByteBit += 8
		}
		byteShift += bitsInCurByte
	}
	return expr, startBit
}

func genUnpackFunc(out io.Writer, maxWidth int, bw int) {
	fmt.Fprintf(out, "func unpack8int%d_%d(data []byte) (a [8]int%d) {\n", maxWidth, bw, maxWidth)
	fmt.Fprintf(out, "\t_ = data[%d]\n", bw-1)
	startBit := 7
	var expr string
	for i := 0; i < 8; i++ {
		expr, startBit = genExpr(maxWidth, bw, i, startBit)
		fmt.Fprintf(out, "\ta[%d] = int%d(%s)\n", i, maxWidth, expr)
	}
	fmt.Fprintf(out, "\treturn\n")
	fmt.Fprintf(out, "}\n\n")
}

func getBits(idx int, bitSize, size, left, pos int, rev bool) string {
	op := "<<"
	if rev {
		op = ">>"
	}
	return fmt.Sprintf("uint%d(data[%d])%s%d", bitSize, idx, op, size-left+pos)
}

func genPackFunc(w io.Writer, bitSize, size int) {
	var (
		left = size
		indx int
		rev  bool
	)

	fmt.Fprintf(w, "func pack8int%[1]d_%[2]d(data [8]int%[1]d) []byte {", bitSize, size)
	fmt.Fprintln(w, "\n\treturn []byte{")
	for i := 0; i < size; i++ {
		var fields []string
		for right := 0; right < 8; {
			if left == 0 {
				indx++
				left = size
				rev = false
			}
			fields = append(fields, getBits(indx, bitSize, size, left, right, rev))
			if left >= 8-right {
				left -= (8 - right)
				right = 8
				rev = true
			} else {
				right += left
				left = 0
			}
		}

		fmt.Fprintf(w, "\t\tbyte(%s),\n", strings.Join(fields, " | "))
	}
	fmt.Fprintln(w, "\t}\n}\n")
}

func funcSlice(bitSize int) string {
	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, `var unpack8Int%[1]dFuncByWidth = [%[2]d]unpack8int%[1]dFunc{`, bitSize, bitSize+1)
	for i := 0; i <= bitSize; i++ {
		fmt.Fprintf(buf, "\n\tunpack8int%d_%d,", bitSize, i)
	}
	fmt.Fprintf(buf, "\n}\n")

	fmt.Fprintf(buf, `var pack8Int%[1]dFuncByWidth = [%[2]d]pack8int%[1]dFunc{`, bitSize, bitSize+1)
	for i := 0; i <= bitSize; i++ {
		fmt.Fprintf(buf, "\n\tpack8int%d_%d,", bitSize, i)
	}
	fmt.Fprintf(buf, "\n}\n")

	return buf.String()
}

func zeroFuncs(w io.Writer, bitSize int) {
	fmt.Fprintf(w, `
type (
	unpack8int%[1]dFunc func([]byte) [8]int%[1]d
	pack8int%[1]dFunc func([8]int%[1]d) []byte
)

%[2]s

func unpack8int%[1]d_0(_ []byte) (a [8]int%[1]d) {
	return a
}

func pack8int%[1]d_0(_ [8]int%[1]d) []byte {
	return []byte{}
}

`, bitSize, funcSlice(bitSize))
}

func genPackage(fn string, maxWidth int) {
	buf := new(bytes.Buffer)

	fmt.Fprint(buf, "// Code generated by \"bitpacking_gen.go\"; DO NOT EDIT.\n\n")
	fmt.Fprintf(buf, "package goparquet\n\n")

	zeroFuncs(buf, maxWidth)
	for i := 1; i <= maxWidth; i++ {
		genUnpackFunc(buf, maxWidth, i)
		genPackFunc(buf, maxWidth, i)
	}

	src, err := format.Source(buf.Bytes())
	if err != nil {
		log.Fatal(err)
	}

	err = ioutil.WriteFile(fn, src, 0644)
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	genPackage("bitbacking32.go", 32)
	genPackage("bitpacking64.go", 64)
}
