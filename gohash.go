/*
Copyright (c) 2014, Gregory L. Dietsche
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

* Redistributions of source code must retain the above copyright notice, this
  list of conditions and the following disclaimer.

* Redistributions in binary form must reproduce the above copyright notice,
  this list of conditions and the following disclaimer in the documentation
  and/or other materials provided with the distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/
package main

import (
	"bufio"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"flag"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
)

var fHash = flag.String("h", "sha256", "valid hashes: crc32, md5, sha1, sha224, sha256, sha384, sha512")
var fConcurrent = flag.Int("j", runtime.NumCPU()*4, "Maximum number of files processed concurrently.")
var fCheck = flag.Bool("c", false, "Read hash from FILE and verify.")

type fileHash struct {
	fileName         *string
	r                io.ReadCloser
	hash             []byte
	expectedHashType *string
	expecteHash      *string
}

//Setup flags and sanitize user input
func handleFlags() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s v1.0 Copyright (c) 2014, Gregory L. Dietsche.\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Usage of %s: [OPTION]... [FILE]...\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if *fConcurrent <= 0 {
		*fConcurrent = 1
	}

	*fHash = strings.ToLower(*fHash)

	runtime.GOMAXPROCS(runtime.NumCPU())
}

//Do your thing
func main() {
	handleFlags()
	in := make(chan fileHash, *fConcurrent*2)
	out := make(chan fileHash, *fConcurrent*2)

	if *fCheck {
		go openFilesForCheck(in)
		go hashFiles(out, in)

		for curResult := range out {
			var computed = fmt.Sprintf("%0x", curResult.hash)
			fmt.Printf("%s %t\n", *curResult.fileName, computed == *curResult.expecteHash)
		}
	} else {
		go openFilesForHashing(in)
		go hashFiles(out, in)

		for curResult := range out {
			if curResult.fileName == nil {
				fmt.Printf("%0x\n", curResult.hash)
			} else {
				fmt.Printf("%s %0x %s\n", *fHash, curResult.hash, *curResult.fileName)
			}
		}
	}
}

func openFilesForCheck(in chan<- fileHash) {
	defer close(in)

	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "Please specify a file that contains previous hash output from this program.")
		return
	}

	checkFile, err := os.Open(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return
	}
	defer checkFile.Close()

	s := bufio.NewScanner(checkFile)
	for s.Scan() {
		var splits = strings.Split(s.Text(), " ")
		if stream, err := os.Open(splits[2]); err == nil {
			in <- fileHash{&splits[2], stream, nil, &splits[0], &splits[1]}
		} else {
			fmt.Fprintln(os.Stderr, err.Error())
		}
	}
}

func openFilesForHashing(in chan<- fileHash) {
	defer close(in)
	if flag.NArg() == 0 {
		in <- fileHash{nil, os.Stdin, nil, fHash, nil}
	} else {
		for i := range flag.Args() {
			file := flag.Arg(i)
			if stream, err := os.Open(file); err == nil {
				in <- fileHash{&file, stream, nil, fHash, nil}
			} else {
				fmt.Fprintln(os.Stderr, err.Error())
			}
		}
	}
}

func hashFiles(out chan<- fileHash, in <-chan fileHash) {
	defer close(out)
	var wg sync.WaitGroup
	for i := 0; i < *fConcurrent; i++ {
		wg.Add(1)
		go digester(&wg, out, in)
	}
	wg.Wait()
}

func digester(wg *sync.WaitGroup, out chan<- fileHash, streams <-chan fileHash) {
	for file := range streams {
		var hash hash.Hash

		switch *file.expectedHashType {
		case "crc32":
			hash = crc32.NewIEEE()
		case "md5":
			hash = md5.New()
		case "sha1":
			hash = sha1.New()
		case "sha224":
			hash = sha256.New224()
		case "sha256":
			hash = sha256.New()
		case "sha384":
			hash = sha512.New384()
		case "sha512":
			hash = sha512.New()
		default:
			fmt.Fprintf(os.Stderr, "%s: I don't know how to compute a %s hash!\n", *file.fileName, *file.expectedHashType)
			file.r.Close()
			continue
		}

		io.Copy(hash, file.r)
		file.r.Close()
		file.hash = hash.Sum(nil)

		out <- file
	}
	wg.Done()
}
