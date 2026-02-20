'use client'

import * as React from 'react'

export const RichTextEditorContext = React.createContext(null)

RichTextEditorContext.displayName = 'RichTextEditorContext'

export function useRichTextEditorContext() {
  const context = React.useContext(RichTextEditorContext)
  if (!context) {
    throw new Error(
      'useRichTextEditorContext must be used within a RichTextEditorRoot',
    )
  }
  return context
}
