'use client'

import { Combobox as ChakraCombobox, Portal } from '@chakra-ui/react'
import { CloseButton } from './close-button'
import * as React from 'react'

export const ComboboxControl = React.forwardRef(
  function ComboboxControl(props, ref) {
    const { children, clearable, ...rest } = props
    return (
      <ChakraCombobox.Control {...rest} ref={ref}>
        {children}
        <ChakraCombobox.IndicatorGroup>
          {clearable && <ComboboxClearTrigger />}
          <ChakraCombobox.Trigger />
        </ChakraCombobox.IndicatorGroup>
      </ChakraCombobox.Control>
    )
  },
)

const ComboboxClearTrigger = React.forwardRef(
  function ComboboxClearTrigger(props, ref) {
    return (
      <ChakraCombobox.ClearTrigger asChild {...props} ref={ref}>
        <CloseButton
          size='xs'
          variant='plain'
          focusVisibleRing='inside'
          focusRingWidth='2px'
          pointerEvents='auto'
        />
      </ChakraCombobox.ClearTrigger>
    )
  },
)

export const ComboboxContent = React.forwardRef(
  function ComboboxContent(props, ref) {
    const { portalled = true, portalRef, ...rest } = props
    return (
      <Portal disabled={!portalled} container={portalRef}>
        <ChakraCombobox.Positioner>
          <ChakraCombobox.Content {...rest} ref={ref} />
        </ChakraCombobox.Positioner>
      </Portal>
    )
  },
)

export const ComboboxItem = React.forwardRef(function ComboboxItem(props, ref) {
  const { item, children, ...rest } = props
  return (
    <ChakraCombobox.Item key={item.value} item={item} {...rest} ref={ref}>
      {children}
      <ChakraCombobox.ItemIndicator />
    </ChakraCombobox.Item>
  )
})

export const ComboboxRoot = React.forwardRef(function ComboboxRoot(props, ref) {
  return (
    <ChakraCombobox.Root
      {...props}
      ref={ref}
      positioning={{ sameWidth: true, ...props.positioning }}
    />
  )
})

export const ComboboxItemGroup = React.forwardRef(
  function ComboboxItemGroup(props, ref) {
    const { children, label, ...rest } = props
    return (
      <ChakraCombobox.ItemGroup {...rest} ref={ref}>
        <ChakraCombobox.ItemGroupLabel>{label}</ChakraCombobox.ItemGroupLabel>
        {children}
      </ChakraCombobox.ItemGroup>
    )
  },
)

export const ComboboxLabel = ChakraCombobox.Label
export const ComboboxInput = ChakraCombobox.Input
export const ComboboxEmpty = ChakraCombobox.Empty
export const ComboboxItemText = ChakraCombobox.ItemText
