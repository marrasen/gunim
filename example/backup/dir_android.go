package main

import "github.com/marrasen/gunim/driver/android"

// noBackupDir is the folder that Android never backs up.
func noBackupDir() (string, error) { return android.NoBackupDir() }
