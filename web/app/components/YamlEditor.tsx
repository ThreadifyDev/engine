import CodeMirror from '@uiw/react-codemirror';
import { yaml } from '@codemirror/lang-yaml';
import { EditorView } from '@codemirror/view';

interface YamlEditorProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  height?: string;
  readOnly?: boolean;
  contractSource?: boolean;
  appearance?: 'default' | 'soft';
}

export default function YamlEditor({
  value,
  onChange,
  placeholder = 'Enter YAML here...',
  height = '400px',
  readOnly = false,
  contractSource = false,
  appearance = 'default',
}: YamlEditorProps) {
  const firstLine = value.split(/\r?\n/).map(line => line.trim()).find(line => line && !line.startsWith("#"));
  const isGherkin = contractSource && firstLine?.startsWith("Feature:");
  return (
    <div className={appearance === 'soft' ? 'overflow-hidden' : 'border-2 border-black'}>
      <CodeMirror
        value={value}
        height={height}
        extensions={[
          ...(isGherkin ? [] : [yaml()]),
          EditorView.lineWrapping,
          EditorView.contentAttributes.of({ 'aria-label': contractSource ? 'Contract source' : 'YAML source' }),
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
