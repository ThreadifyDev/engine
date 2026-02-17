import CodeMirror from '@uiw/react-codemirror';
import { yaml } from '@codemirror/lang-yaml';
import { EditorView } from '@codemirror/view';

interface YamlEditorProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  height?: string;
  readOnly?: boolean;
}

export default function YamlEditor({
  value,
  onChange,
  placeholder = 'Enter YAML here...',
  height = '400px',
  readOnly = false,
}: YamlEditorProps) {
  return (
    <div className="border-2 border-black">
      <CodeMirror
        value={value}
        height={height}
        extensions={[
          yaml(),
          EditorView.lineWrapping,
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
