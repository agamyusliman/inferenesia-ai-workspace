package brand

import (
	"strings"
	"testing"
)

func TestProductNameIsInferenesia(t *testing.T) {
	if Name != "Inferenesia" {
		t.Fatalf("Name = %q, want Inferenesia", Name)
	}
	if strings.Contains(Name, "temp-ai") {
		t.Fatalf("product Name must not contain temp-ai: %q", Name)
	}
	if Name == "Infernesia" || strings.EqualFold(Name, "infernesia") {
		t.Fatalf("product Name must not be typo brand Infernesia: %q", Name)
	}
	// VAL-CROSS brand: product must never be the forbidden "temp-ai Agent" name.
	if strings.Contains(Name, "temp-ai Agent") {
		t.Fatalf("product Name uses forbidden brand: %q", Name)
	}
	if Binary != "inferenesia" {
		t.Fatalf("Binary = %q, want inferenesia", Binary)
	}
	if DesktopBinary != "inferenesia-desktop" {
		t.Fatalf("DesktopBinary = %q, want inferenesia-desktop", DesktopBinary)
	}
}

func TestProviderLabelsAreDistinctFromProduct(t *testing.T) {
	if ProviderGateway != "Inferenesia API" {
		t.Fatalf("ProviderGateway = %q, want Inferenesia API", ProviderGateway)
	}
	// ProviderTempAI stays as a deprecated alias constant for legacy compat.
	if ProviderTempAI != "temp-ai" {
		t.Fatalf("ProviderTempAI = %q, want temp-ai (deprecated alias)", ProviderTempAI)
	}
	// Product strings must never be the old brand "temp-ai Agent".
	for _, s := range []string{Name, ShortDescription, StartupBanner, Binary, DesktopBinary, ProviderGateway} {
		if strings.Contains(s, "temp-ai Agent") {
			t.Fatalf("product string uses forbidden brand: %q", s)
		}
	}
	// ProviderGateway must not equal the product Name (label ≠ product).
	if ProviderGateway == Name {
		t.Fatalf("ProviderGateway must differ from Name: %q", Name)
	}
}

func TestStartupBannerIncludesName(t *testing.T) {
	if !strings.Contains(StartupBanner, Name) {
		t.Fatalf("StartupBanner %q must contain Name %q", StartupBanner, Name)
	}
}
