import CodeMirror from '@uiw/react-codemirror';
import { EditorView } from '@codemirror/view';

interface ContractEditorProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  height?: string;
  readOnly?: boolean;
  appearance?: 'default' | 'soft';
}

export default function ContractEditor({
  value,
  onChange,
  placeholder = 'Feature: your_workflow',
  height = '400px',
  readOnly = false,
  appearance = 'default',
}: ContractEditorProps) {
  return (
    <div className={appearance === 'soft' ? 'overflow-hidden' : 'border-2 border-black'}>
      <CodeMirror
        value={value}
        height={height}
        extensions={[
          EditorView.lineWrapping,
          EditorView.contentAttributes.of({ 'aria-label': 'Contract source' }),
          ...(appearance === 'soft' ? [EditorView.theme({
            '&': { backgroundColor: '#fff' },
            '&.cm-focused': { outline: 'none' },
            '.cm-scroller': { fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace', lineHeight: '1.8' },
            '.cm-content': { padding: '20px 0' },
            '.cm-line': { padding: '0 16px' },
            '.cm-gutters': { backgroundColor: '#fafaf9', color: '#a8a29e', border: 'none', padding: '0 4px 0 8px' },
            '.cm-activeLine, .cm-activeLineGutter': { backgroundColor: '#f5f7f4' },
          })] : []),
        ]}
        onChange={onChange}
        placeholder={placeholder}
        readOnly={readOnly}
        basicSetup={{
          lineNumbers: true,
          highlightActiveLineGutter: true,
          highlightSpecialChars: true,
          foldGutter: true,
          drawSelection: true,
          dropCursor: true,
          allowMultipleSelections: true,
          indentOnInput: true,
          syntaxHighlighting: true,
          bracketMatching: true,
          closeBrackets: true,
          autocompletion: true,
          rectangularSelection: true,
          crosshairCursor: true,
          highlightActiveLine: true,
          highlightSelectionMatches: true,
          closeBracketsKeymap: true,
          searchKeymap: true,
          foldKeymap: true,
          completionKeymap: true,
          lintKeymap: true,
        }}
        theme="light"
        style={{
          fontSize: '14px',
          fontFamily: 'Monaco, Menlo, "Ubuntu Mono", Consolas, monospace',
        }}
      />
    </div>
  );
}
