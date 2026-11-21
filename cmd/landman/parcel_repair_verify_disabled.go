//go:build !parcelrepairverify

package main

import "context"

func checkParcelRepairVerificationMode(context.Context, string, string) error { return nil }
