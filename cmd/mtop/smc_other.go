//go:build !darwin || !cgo

package main

func smcTemps() (cpu, gpu float64) { return 0, 0 }
