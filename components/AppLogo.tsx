import React from 'react';

interface AppLogoProps {
  className?: string;
}

export const AppLogo: React.FC<AppLogoProps> = ({ className }) => (
  <img
    src="./icon.png"
    alt=""
    draggable={false}
    className={className}
  />
);

export default AppLogo;
