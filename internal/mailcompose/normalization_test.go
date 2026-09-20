package mailcompose

import "testing"

func TestInlineDispositionNormalization(t *testing.T) {
	in := validInput()
	in.Content = Content{Format: FormatHTML, HTML: `<p>Hello<img src="cid:logo@example.com"></p>`}

	in.Attachments = []Attachment{{Filename: "logo.png", ContentType: "image/png", Data: []byte("png"), Disposition: " inline ", ContentID: "logo@example.com"}}
	if _, err := Compose(in); err != nil {
		t.Fatal(err)
	}
}

func TestCSSRejectsNegativeDimensions(t *testing.T) {
	for _, value := range []string{"padding:-1px", "width:-2px", "border-width:-3px", "font-size:-4px", "border:-1px solid red", "line-height:-1px"} {
		if _, _, err := parseStyleAttribute(value); err == nil {
			t.Errorf("accepted %s", value)
		}
	}

	if _, _, err := parseStyleAttribute("margin:-1px;letter-spacing:-1px"); err != nil {
		t.Fatal(err)
	}
}

func TestSafeCSSValuesDoNotAbort(t *testing.T) {
	if _, _, err := parseStyleAttribute("font-family:'Behavioral Sans'"); err != nil {
		t.Fatal(err)
	}

	styles, warnings, err := parseStyleAttribute("color:orange")
	if err != nil || len(styles) == 0 && len(warnings) == 0 {
		t.Fatalf("styles=%#v warnings=%#v err=%v", styles, warnings, err)
	}
}

func TestImageOnlyPlainFallback(t *testing.T) {
	in := validInput()
	in.Content = Content{Format: FormatHTML, HTML: `<img src="https://images.example/logo.png">`}

	out, err := Compose(in)
	if err != nil {
		t.Fatal(err)
	}

	if out.Summary.Preview.Plain != "[Image]" {
		t.Fatalf("plain fallback: %q", out.Summary.Preview.Plain)
	}
}
