interface CodeBlockProps {
  code: string;
  title?: string;
  language?: string;
  headerColor?: "gray" | "purple" | "blue" | "green";
}

const headerColors = {
  gray: "bg-gray-800 border-gray-700",
  purple: "bg-purple-800 border-purple-700",
  blue: "bg-blue-800 border-blue-700",
  green: "bg-green-800 border-green-700",
};

const titleColors = {
  gray: "text-gray-300",
  purple: "text-purple-100",
  blue: "text-blue-100",
  green: "text-green-100",
};

export default function CodeBlock({ 
  code, 
  title, 
  language = "javascript",
  headerColor = "gray" 
}: CodeBlockProps) {
  return (
    <div className="bg-white rounded-xl shadow-lg border border-gray-200 overflow-hidden">
      {title && (
        <div className={`px-6 py-3 border-b ${headerColors[headerColor]}`}>
          <h4 className={`text-sm font-semibold ${titleColors[headerColor]}`}>
            {title}
          </h4>
        </div>
      )}
      <div className="p-6">
        <pre className="text-xs text-gray-800 whitespace-pre-wrap break-words">
          <code className="block">{code}</code>
        </pre>
      </div>
    </div>
  );
}
