import React from 'react';

interface AppWordmarkProps {
  accessibleLabel?: string;
  className?: string;
}

/** Product wordmark. Keep the accessible name in sync with the visible brand. */
export const AppWordmark: React.FC<AppWordmarkProps> = ({ accessibleLabel = "LemonSSH", className }) => (
  <span
    aria-label={accessibleLabel}
    className={className}
    role="img"
    style={{
      fontFamily: '"Mona Sans", system-ui, sans-serif',
      fontWeight: 800,
      fontStyle: "italic",
      letterSpacing: "-0.04em",
      lineHeight: 1,
      display: "inline-block",
    }}
  >
    LemonSSH
  </span>
);

export default AppWordmark;
