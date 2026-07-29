//go:build !linux

package archive

func Publish(stage, destination string) error {
	return publishFallback(stage, destination)
}
