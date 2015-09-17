// Copyright 2015 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package overlord

type GhostRPC struct {
	ghost *Ghost
}

type EmptyArgs struct {
}

type EmptyReply struct {
}

func NewGhostRPC(ghost *Ghost) *GhostRPC {
	return &GhostRPC{ghost}
}

func (self *GhostRPC) Reconnect(args *EmptyArgs, reply *EmptyReply) error {
	self.ghost.reset = true
	return nil
}

func (self *GhostRPC) GetStatus(args *EmptyArgs, reply *string) error {
	*reply = self.ghost.RegisterStatus
	return nil
}

func (self *GhostRPC) RegisterTTY(args []string, reply *EmptyReply) error {
	self.ghost.RegisterTTY(args[0], args[1])
	return nil
}

func (self *GhostRPC) RegisterSession(args []string, reply *EmptyReply) error {
	self.ghost.RegisterSession(args[0], args[1])
	return nil
}

func (self *GhostRPC) AddToDownloadQueue(args []string, reply *EmptyReply) error {
	self.ghost.AddToDownloadQueue(args[0], args[1])
	return nil
}
