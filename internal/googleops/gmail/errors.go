package gmail

import (
	nativegoogleapi "github.com/steipete/gogcli/internal/googleapi"
)

func publicError(err error) error {
	if err == nil {
		return nil
	}

	return nativegoogleapi.NativePublicError(err)
}
