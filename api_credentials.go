package main

import "github.com/zalando/go-keyring"

func deleteAPICredential(service, id string) error { return keyring.Delete(service, id) }
