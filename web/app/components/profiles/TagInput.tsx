import { useState } from 'react';
import { X, Database } from 'lucide-react';

export interface TagInputProps {
  tags: string[];
  persistedTags?: string[];
  onChange: (tags: string[]) => void;
  placeholder?: string;
}

export function TagInput({ tags, persistedTags = [], onChange, placeholder }: TagInputProps) {
  const [inputValue, setInputValue] = useState('');

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter' || e.key === ',' || e.key === ' ') {
      e.preventDefault();
      const val = inputValue.trim().replace(/^,/, '');
      if (val && !tags.includes(val)) {
        onChange([...tags, val]);
      }
      setInputValue('');
    } else if (e.key === 'Backspace' && !inputValue && tags.length > 0) {
      const lastTag = tags[tags.length - 1];
      if (!persistedTags.includes(lastTag)) {
        onChange(tags.slice(0, -1));
      }
    }
  };

  const removeTag = (tagToRemove: string) => {
    if (persistedTags.includes(tagToRemove)) return;
    onChange(tags.filter(t => t !== tagToRemove));
  };

  return (
    <div className="w-full px-3 py-2 border border-gray-300 rounded focus-within:ring-1 focus-within:ring-black focus-within:border-black bg-white flex flex-wrap gap-2 items-center min-h-[42px]">
      {tags.map(tag => {
        const isPersisted = persistedTags.includes(tag);
        return (
          <span 
            key={tag} 
            className={`inline-flex items-center gap-1 px-2 py-0.5 rounded text-sm font-mono ${
              isPersisted ? 'bg-gray-100 text-gray-500 border border-gray-200' : 'bg-gray-900 text-white'
            }`}
          >
            {tag}
            {!isPersisted && (
              <button
                type="button"
                onClick={() => removeTag(tag)}
                className="hover:text-red-400 focus:outline-none ml-1"
              >
                <X className="w-3 h-3" />
              </button>
            )}
            {isPersisted && (
               <span className="w-3 h-3 flex items-center justify-center opacity-40">
                 <Database className="w-2.5 h-2.5" />
               </span>
            )}
          </span>
        );
      })}
      <input
        type="text"
        value={inputValue}
        onChange={e => setInputValue(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder={tags.length === 0 ? placeholder : ''}
        className="flex-1 outline-none min-w-[120px] text-sm bg-transparent"
      />
    </div>
  );
}
