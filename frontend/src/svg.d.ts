/* eslint-disable @typescript-eslint/consistent-type-imports */
declare module '*?solid' {
  const src: import('solid-js').Component<import('solid-js').ComponentProps<'svg'>>;
  export default src;
}

declare module '@material-symbols/svg-400/outlined/*.svg?solid' {
  const src: import('solid-js').Component<import('solid-js').ComponentProps<'svg'>>;
  export default src;
}

// Make SolidSVG available globally
type SolidSVG = import('solid-js').Component<import('solid-js').ComponentProps<'svg'>>;
