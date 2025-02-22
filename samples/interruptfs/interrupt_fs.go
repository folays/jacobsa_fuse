// Copyright 2015 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package interruptfs

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/folays/jacobsa_fuse"
	"github.com/folays/jacobsa_fuse/fuseops"
	"github.com/folays/jacobsa_fuse/fuseutil"
)

var rootAttrs = fuseops.InodeAttributes{
	Nlink: 1,
	Mode:  os.ModeDir | 0777,
}

const fooID = fuseops.RootInodeID + 1

var fooAttrs = fuseops.InodeAttributes{
	Nlink: 1,
	Mode:  0777,
	Size:  1234,
}

// A file system containing exactly one file, named "foo". ReadFile and
// FlushFile ops can be made to hang until interrupted. Exposes a method for
// synchronizing with the arrival of a read or a flush.
//
// Must be created with New.
type InterruptFS struct {
	fuseutil.NotImplementedFileSystem

	mu sync.Mutex

	blockForReads   bool // GUARDED_BY(mu)
	blockForFlushes bool // GUARDED_BY(mu)

	// Must hold the mutex when closing these.
	readReceived  chan struct{}
	flushReceived chan struct{}
}

func New() *InterruptFS {
	return &InterruptFS{
		readReceived:  make(chan struct{}),
		flushReceived: make(chan struct{}),
	}
}

////////////////////////////////////////////////////////////////////////
// Public interface
////////////////////////////////////////////////////////////////////////

// Block until the first read is received.
func (fs *InterruptFS) WaitForFirstRead() {
	<-fs.readReceived
}

// Block until the first flush is received.
func (fs *InterruptFS) WaitForFirstFlush() {
	<-fs.flushReceived
}

// Enable blocking until interrupted for the next (and subsequent) read ops.
func (fs *InterruptFS) EnableReadBlocking() {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	fs.blockForReads = true
}

// Enable blocking until interrupted for the next (and subsequent) flush ops.
func (fs *InterruptFS) EnableFlushBlocking() {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	fs.blockForFlushes = true
}

////////////////////////////////////////////////////////////////////////
// FileSystem methods
////////////////////////////////////////////////////////////////////////

func (fs *InterruptFS) StatFS(
	ctx context.Context,
	op *fuseops.StatFSOp) error {
	return nil
}

func (fs *InterruptFS) LookUpInode(
	ctx context.Context,
	op *fuseops.LookUpInodeOp) error {
	// We support only one parent.
	if op.Parent != fuseops.RootInodeID {
		return fmt.Errorf("Unexpected parent: %v", op.Parent)
	}

	// We support only one name.
	if op.Name != "foo" {
		return fuse.ENOENT
	}

	// Fill in the response.
	op.Entry.Child = fooID
	op.Entry.Attributes = fooAttrs

	return nil
}

func (fs *InterruptFS) GetInodeAttributes(
	ctx context.Context,
	op *fuseops.GetInodeAttributesOp) error {
	switch op.Inode {
	case fuseops.RootInodeID:
		op.Attributes = rootAttrs

	case fooID:
		op.Attributes = fooAttrs

	default:
		return fmt.Errorf("Unexpected inode ID: %v", op.Inode)
	}

	return nil
}

func (fs *InterruptFS) OpenFile(
	ctx context.Context,
	op *fuseops.OpenFileOp) error {
	return nil
}

func (fs *InterruptFS) ReadFile(
	ctx context.Context,
	op *fuseops.ReadFileOp) error {
	fs.mu.Lock()
	shouldBlock := fs.blockForReads

	// Signal that a read has been received, if this is the first.
	select {
	case <-fs.readReceived:
	default:
		close(fs.readReceived)
	}
	fs.mu.Unlock()

	// Wait for cancellation if enabled.
	if shouldBlock {
		done := ctx.Done()
		if done == nil {
			panic("Expected non-nil channel.")
		}

		<-done
		return ctx.Err()
	}

	return nil
}

func (fs *InterruptFS) FlushFile(
	ctx context.Context,
	op *fuseops.FlushFileOp) error {
	fs.mu.Lock()
	shouldBlock := fs.blockForFlushes

	// Signal that a flush has been received, if this is the first.
	select {
	case <-fs.flushReceived:
	default:
		close(fs.flushReceived)
	}
	fs.mu.Unlock()

	// Wait for cancellation if enabled.
	if shouldBlock {
		done := ctx.Done()
		if done == nil {
			panic("Expected non-nil channel.")
		}

		<-done
		return ctx.Err()
	}

	return nil
}
