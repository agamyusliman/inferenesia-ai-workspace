package config

import "testing"

func TestNormalizedAPIFormat(t *testing.T) {
	cases := []struct {
		name string
		p    ProviderProfile
		want string
	}{
		{name: "empty type and format defaults to openai", p: ProviderProfile{}, want: APIFormatOpenAI},
		{name: "type openai_compatible empty format", p: ProviderProfile{Type: "openai_compatible"}, want: APIFormatOpenAI},
		{name: "type openai_compatible format openai", p: ProviderProfile{Type: "openai_compatible", APIFormat: "openai"}, want: APIFormatOpenAI},
		{name: "type openai_compatible format anthropic overrides", p: ProviderProfile{Type: "openai_compatible", APIFormat: "anthropic"}, want: APIFormatAnthropic},
		{name: "type anthropic empty format derives anthropic", p: ProviderProfile{Type: "anthropic"}, want: APIFormatAnthropic},
		{name: "type anthropic format openai overrides", p: ProviderProfile{Type: "anthropic", APIFormat: "openai"}, want: APIFormatOpenAI},
		{name: "type claude derives anthropic", p: ProviderProfile{Type: "claude"}, want: APIFormatAnthropic},
		{name: "format claude normalizes to anthropic", p: ProviderProfile{Type: "openai_compatible", APIFormat: "claude"}, want: APIFormatAnthropic},
		{name: "format openai_compatible normalizes to openai", p: ProviderProfile{APIFormat: "openai_compatible"}, want: APIFormatOpenAI},
		{name: "format openai-compatible normalizes to openai", p: ProviderProfile{APIFormat: "openai-compatible"}, want: APIFormatOpenAI},
		{name: "whitespace trimmed", p: ProviderProfile{Type: "  Anthropic  ", APIFormat: "  OpenAI  "}, want: APIFormatOpenAI},
		{name: "unknown format falls back to type", p: ProviderProfile{Type: "anthropic", APIFormat: "gemini"}, want: APIFormatAnthropic},
		{name: "unknown format with openai type falls back to openai", p: ProviderProfile{Type: "openai_compatible", APIFormat: "gemini"}, want: APIFormatOpenAI},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.p.NormalizedAPIFormat()
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestAPIFormatConstants(t *testing.T) {
	if APIFormatOpenAI != "openai" {
		t.Fatalf("APIFormatOpenAI=%q", APIFormatOpenAI)
	}
	if APIFormatAnthropic != "anthropic" {
		t.Fatalf("APIFormatAnthropic=%q", APIFormatAnthropic)
	}
}
