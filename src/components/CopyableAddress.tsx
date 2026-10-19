import { ContentCopy } from '@mui/icons-material';
import { Box, IconButton, Tooltip, Typography } from '@mui/material';
import { useSnackbar } from 'notistack';
import { shortenAddress } from 'utils/formatters';
import { useT } from 'i18n';

// Component for address with copy functionality
interface CopyableAddressProps {
  address: string;
  symbol?: string;
  onClick?: (e: React.MouseEvent) => void;
}

export const CopyableAddress = ({ address, symbol, onClick }: CopyableAddressProps) => {
  const { enqueueSnackbar } = useSnackbar();
  const t = useT();

  const copyToClipboard = (e: React.MouseEvent) => {
    e.stopPropagation(); // Prevent row click event
    navigator.clipboard
      .writeText(address)
      .then(() => {
        enqueueSnackbar(t('copy.success'), {
          variant: 'success',
          autoHideDuration: 2000
        });
      })
      .catch((err) => {
        console.error('Failed to copy address:', err);
        enqueueSnackbar(t('copy.fail'), {
          variant: 'error'
        });
      });
  };
  return (
    <Box
      sx={{
        display: 'flex',
        alignItems: 'center',
        '&:hover .copy-icon, &:focus-within .copy-icon': {
          opacity: 1
        }
      }}
      onClick={onClick}
    >
      {/* the full address is in the title and for assistive technology; the screen shows it shortened */}
      <Typography component="span" title={symbol ? undefined : address}>
        {symbol ? symbol : shortenAddress(address)}
      </Typography>
      <Tooltip title={t('copy.tooltip')} describeChild>
        <IconButton
          size="small"
          onClick={copyToClipboard}
          aria-label={t('copy.aria', { address })}
          className="copy-icon"
          sx={{ ml: 0.5, opacity: 0.3, transition: 'opacity 0.2s', '&:hover, &:focus-visible': { opacity: 1 } }}
        >
          <ContentCopy fontSize="small" />
        </IconButton>
      </Tooltip>
    </Box>
  );
};
