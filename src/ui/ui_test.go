package ui

import "testing"

func TestVersionVendoredAssets(t *testing.T) {
	cases := map[string]string{
		`<script src="js/tailwind.min.js"></script>`:             `<script src="js/tailwind.min.js?v=1.2.3"></script>`,
		`<script src="js/babel.min.js"></script>`:                `<script src="js/babel.min.js?v=1.2.3"></script>`,
		`import { React } from './js/react-bundle.esm.js';`:      `import { React } from './js/react-bundle.esm.js?v=1.2.3';`,
		`<script src="js/babel-config.js"></script>`:             `<script src="js/babel-config.js"></script>`,
		`import * as FloatingUI from './js/floating-ui.esm.js';`: `import * as FloatingUI from './js/floating-ui.esm.js';`,
	}
	for in, want := range cases {
		if got := versionVendoredAssets(in, "1.2.3"); got != want {
			t.Errorf("versionVendoredAssets(%q) = %q, want %q", in, got, want)
		}
	}
}
