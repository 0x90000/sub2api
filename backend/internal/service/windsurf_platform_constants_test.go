package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/model"
)

func TestWindsurfPlatformConstants(t *testing.T) {
	if PlatformWindsurf != domain.PlatformWindsurf {
		t.Fatalf("PlatformWindsurf = %q, want %q", PlatformWindsurf, domain.PlatformWindsurf)
	}

	platforms := model.AllPlatforms()
	for _, platform := range platforms {
		if platform == PlatformWindsurf {
			return
		}
	}

	t.Fatalf("model.AllPlatforms() = %v, want to contain %q", platforms, PlatformWindsurf)
}
