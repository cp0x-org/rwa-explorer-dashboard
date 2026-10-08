import Box, { BoxProps } from '@mui/material/Box';
import { Avatar, AvatarProps } from '@mui/material';
import React from 'react';

interface TokenIconProps extends BoxProps {
  symbol: string;
  avatarProps?: AvatarProps; // чтобы можно было кастомизировать Avatar
}

// Placeholder icon: renders a letter avatar from the token symbol.
// Swap in real icon sources (e.g. /tokens/<symbol>.svg) when wiring a protocol.
export const TokenIcon: React.FC<TokenIconProps> = ({ symbol, avatarProps, ...boxProps }) => {
  if (!symbol) {
    return null;
  }

  return (
    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }} {...boxProps}>
      <Avatar alt={`${symbol} icon`} sx={{ width: 36, height: 36, fontSize: 14, ...avatarProps?.sx }} {...avatarProps}>
        {symbol.slice(0, 2).toUpperCase()}
      </Avatar>
    </Box>
  );
};
