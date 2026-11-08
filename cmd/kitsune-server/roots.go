package main

// The image is FROM scratch, with no CA certificates, so without roots of its
// own every HTTPS call (Last.fm) fails with "certificate signed by unknown
// authority". This bundles Mozilla's roots, used only when the system has none.
import _ "golang.org/x/crypto/x509roots/fallback"
