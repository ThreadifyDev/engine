import { ReactNode } from "react";

interface CodeBlockProps {
  code: string;
  title?: string;
  language?: string;
  headerColor?: "gray" | "purple" | "blue" | "green";
}

const highlightCode = (code: string) => {
  let tokens: string[] = [];

  const tokenize = (html: string) => {
    tokens.push(html);
    return `__TOKEN_${tokens.length - 1}__`;
  };

  let processed = code
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");

  // 1. Strings
  processed = processed.replace(/(["'])(.*?)\1/g, (match) => tokenize(`<span class="text-emerald-300">${match}</span>`));

  // 2. Comments
  processed = processed.replace(/(\/\/.*)/g, (match) => tokenize(`<span class="text-slate-500 italic">${match}</span>`));

  // 3. Method calls
  processed = processed.replace(/\.(\w+)(\s*\()/g, (_, method, paren) => {
    return `.${tokenize(`<span class="text-blue-400">${method}</span>`)}${paren}`;
  });

  // 4. Object keys
  processed = processed.replace(/([a-zA-Z0-9_]+):/g, (_, key) => {
    return `${tokenize(`<span class="text-indigo-300">${key}</span>`)}:`;
  });

  // 5. Keywords
  processed = processed.replace(/\b(const|await|async|let|var|function|return|import|from)\b/g, (match) => tokenize(`<span class="text-pink-400">${match}</span>`));

  // 6. Known variables
  processed = processed.replace(/\b(threadify|thread|step|connection|notification|console|data|invitation|invitationToken)\b/g, (match) => tokenize(`<span class="text-sky-300">${match}</span>`));

  // Restore tokens
  tokens.forEach((token, i) => {
    processed = processed.replace(`__TOKEN_${i}__`, token);
  });

  return <code className="block font-mono text-[13px] leading-[1.6]" dangerouslySetInnerHTML={{ __html: processed }} />;
};

function dedent(str: string) {
  const lines = str.split('\n');
  let minIndent = Infinity;
  for (const line of lines) {
    if (line.trim().length === 0) continue;
    const match = line.match(/^\s*/);
    if (match) {
      minIndent = Math.min(minIndent, match[0].length);
    }
  }
  if (minIndent === Infinity) return str.trim();
  return lines.map(line => line.slice(minIndent)).join('\n').trim();
}

const bgColors = {
  gray: "from-[#1e1e24] to-[#0f0f13]",
  purple: "from-[#2a1e35] to-[#0f0f13]",
  blue: "from-[#1e2735] to-[#0f0f13]",
  green: "from-[#1e3025] to-[#0f0f13]",
};

const borderColors = {
  gray: "border-white/10",
  purple: "border-purple-500/20",
  blue: "border-blue-500/20",
  green: "border-emerald-500/20",
};

export default function CodeBlock({ 
  code, 
  title, 
  language = "javascript",
  headerColor = "gray" 
}: CodeBlockProps) {
  return (
    <div className={`relative rounded-2xl overflow-hidden bg-gradient-to-br ${bgColors[headerColor]} border ${borderColors[headerColor]} shadow-2xl`}>
      {/* Glossy top edge highlight */}
      <div className="absolute top-0 left-0 right-0 h-px bg-gradient-to-r from-transparent via-white/10 to-transparent" />
      
      {/* Header bar */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-white/5 bg-black/20">
        <div className="flex items-center gap-3">
          {/* macOS window controls */}
          <div className="flex items-center gap-1.5 mr-2">
            <div className="w-2.5 h-2.5 rounded-full bg-red-500/80" />
            <div className="w-2.5 h-2.5 rounded-full bg-amber-500/80" />
            <div className="w-2.5 h-2.5 rounded-full bg-emerald-500/80" />
          </div>
          {title && (
            <span className="text-xs font-semibold text-slate-300 font-sans tracking-wide">
              {title}
            </span>
          )}
        </div>
        {language && (
          <span className="text-[10px] font-mono text-slate-500 uppercase tracking-widest">
            {language}
          </span>
        )}
      </div>

      {/* Code body */}
      <div className="p-5 overflow-x-auto text-slate-200">
        <pre className="m-0">
          {highlightCode(dedent(code))}
        </pre>
      </div>
    </div>
  );
}
