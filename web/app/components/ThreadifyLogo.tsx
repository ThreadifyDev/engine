interface ThreadifyLogoProps {
  className?: string;
  height?: number;
}

export default function ThreadifyLogo({ className = "", height = 28 }: ThreadifyLogoProps) {
  const scale = height / 30;
  const textSize = 18.4 * scale;

  return (
    <svg
      height={height}
      viewBox="0 0 160 30"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
      aria-label="Threadify"
    >
      {/* Needle T */}
      <g transform="scale(1.3) translate(0, 1)">
        <path d="M8 20 L7.2 5" stroke="currentColor" strokeWidth="2" strokeLinecap="round"/>
        <path d="M8 20 L8.8 5" stroke="currentColor" strokeWidth="2" strokeLinecap="round"/>
        {/* needle eye */}
        <ellipse cx="8" cy="4" rx="1.4" ry="2" stroke="currentColor" strokeWidth="1.4" fill="none"/>
        {/* thread crossbar */}
        <path d="M1 3.5 Q4 1.5 8 2.5 Q12 3.5 15 1.5" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" fill="none"/>
      </g>
      {/* hreadify text */}
      <text
        x="22"
        y="22"
        fontFamily="-apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
        fontSize={textSize}
        fontWeight="600"
        letterSpacing="-0.3"
        fill="currentColor"
      >
        hreadify
      </text>
    </svg>
  );
}
