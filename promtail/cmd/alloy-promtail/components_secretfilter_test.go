//go:build promtail_secretfilter

package main

func init() {
	expectedComponents = append(expectedComponents, "loki.secretfilter")
}
