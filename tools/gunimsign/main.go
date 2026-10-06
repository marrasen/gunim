// Command gunimsign makes the key a program's releases are signed with, and signs a release's SHA256SUMS with it, for
// the updates of gunim's install package.
//
//	gunimsign -keygen release.key
//
// writes a new private key to release.key and prints its public key, for install.App.UpdateKey.
//
//	GUNIM_SIGN_KEY=$(cat release.key) gunimsign dist/SHA256SUMS
//
// writes dist/SHA256SUMS.sig, the signature an installed program checks before it runs an update. -key reads the
// private key from a file in place of GUNIM_SIGN_KEY.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

func main() {
	keygen := flag.String("keygen", "", "write a new private key to this file, and print its public key")
	keyFile := flag.String("key", "", "the file of the private key; $GUNIM_SIGN_KEY when empty")
	flag.Parse()
	if *keygen != "" {
		public, err := makeKey(*keygen)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(public)
		return
	}
	if flag.NArg() != 1 {
		log.Fatal("gunimsign: give the SHA256SUMS to sign, or -keygen")
	}
	text := os.Getenv("GUNIM_SIGN_KEY")
	if *keyFile != "" {
		raw, err := os.ReadFile(*keyFile)
		if err != nil {
			log.Fatal(err)
		}
		text = string(raw)
	}
	key, err := readKey(text)
	if err != nil {
		log.Fatal(err)
	}
	if err := signFile(key, flag.Arg(0)); err != nil {
		log.Fatal(err)
	}
}

// makeKey writes a new private key to path, readable by its owner alone, and returns the public key.
func makeKey(path string) (string, error) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := fmt.Fprintln(f, base64.StdEncoding.EncodeToString(private.Seed())); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(public), nil
}

// readKey reads a private key as makeKey writes it.
func readKey(text string) (ed25519.PrivateKey, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("gunimsign: no private key: set GUNIM_SIGN_KEY or -key")
	}
	seed, err := base64.StdEncoding.DecodeString(text)
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("gunimsign: the private key is not one gunimsign -keygen wrote")
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// signFile writes path.sig, key's signature of the file at path, as install checks it.
func signFile(key ed25519.PrivateKey, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(key, raw)) + "\n"
	return os.WriteFile(path+".sig", []byte(sig), 0o644)
}
