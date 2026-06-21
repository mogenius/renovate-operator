// Babel 8 defaults to the automatic JSX runtime, which generates
// `import { jsx } from "react/jsx-runtime"` in compiled output.
// That `import` statement causes a browser error when Babel injects
// the compiled script as a non-module <script> tag.
// Override to classic mode so JSX compiles to React.createElement calls.
(function () {
  var origReact = Babel.availablePresets['react'];
  Babel.registerPreset('react', {
    presets: [[origReact, { runtime: 'classic' }]]
  });
})();
