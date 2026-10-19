import Box, { BoxProps } from '@mui/material/Box';
import { Avatar, AvatarProps } from '@mui/material';
import React from 'react';

interface TokenIconProps extends BoxProps {
  symbol: string;
  src?: string; // logo image; the letters stay as fallback if it fails to load
  avatarProps?: AvatarProps; // чтобы можно было кастомизировать Avatar
}

// Logo when available, otherwise a letter avatar from the token symbol.
export const TokenIcon: React.FC<TokenIconProps> = ({ symbol, src, avatarProps, ...boxProps }) => {
  if (!symbol) {
    return null;
  }
  const { sx: avatarSx, ...avatarRest } = avatarProps ?? {};

  return (
    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }} {...boxProps}>
      {/* decorative: the symbol is written next to it; the letter fallback keeps AA contrast on both themes */}
      <Avatar
        alt=""
        src={src}
        sx={{ width: 36, height: 36, fontSize: 14, bgcolor: '#3f3f3f', color: '#ffffff', ...avatarSx }}
        {...avatarRest}
      >
        {symbol.slice(0, 2).toUpperCase()}
      </Avatar>
    </Box>
  );
};
