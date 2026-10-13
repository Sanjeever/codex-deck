//go:build darwin || linux

package main

import "context"

func prepareHiddenWindow(context.Context) error { return nil }
