declare module '*.svg?solid' {
  import { Component, ComponentProps } from 'solid-js';
  const src: Component<ComponentProps<'svg'>>;
  export default src;
}

// Make SolidSVG available globally
import { Component, ComponentProps } from 'solid-js';
declare global {
  type SolidSVG = Component<ComponentProps<'svg'>>;
}
