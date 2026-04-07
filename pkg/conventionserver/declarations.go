package conventionserver

import "github.com/campfire-net/ready/pkg/declarations"

// loadDeclData loads raw declaration JSON for the given operation name.
// Delegates to the embedded declarations package.
func loadDeclData(name string) ([]byte, error) {
	return declarations.Load(name)
}
