// Shared text button foundation for primary, secondary, ghost, and unstyled controls.

import { splitProps, type JSX } from 'solid-js';
import styles from './Button.module.css';

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'unstyled';

export interface ButtonProps extends JSX.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
}

export function Button(props: ButtonProps) {
  const [local, buttonProps] = splitProps(props, ['class', 'type', 'variant']);
  const variant = () => local.variant ?? 'secondary';
  const type = () => local.type ?? 'button';
  const variantClass = () => {
    switch (variant()) {
      case 'primary':
        return styles.primary;
      case 'ghost':
        return styles.ghost;
      case 'unstyled':
        return styles.unstyled;
      case 'secondary':
        return styles.secondary;
    }
  };

  return <button {...buttonProps} class={`${styles.button} ${variantClass()} ${local.class ?? ''}`} type={type()} />;
}
