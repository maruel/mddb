// Shared accessible icon-only button foundation.

import { splitProps } from 'solid-js';
import { Button, type ButtonProps } from './Button';
import styles from './IconButton.module.css';

export interface IconButtonProps extends ButtonProps {
  'aria-label': string;
}

export function IconButton(props: IconButtonProps) {
  const [local, buttonProps] = splitProps(props, ['class']);
  return <Button {...buttonProps} class={`${styles.iconButton} ${local.class ?? ''}`} />;
}
