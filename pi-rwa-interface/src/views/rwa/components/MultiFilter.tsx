import { useMemo } from 'react';
import Autocomplete from '@mui/material/Autocomplete';
import Chip from '@mui/material/Chip';
import TextField from '@mui/material/TextField';

import { useLocale } from 'i18n';

interface MultiFilterProps {
  label: string;
  options: string[];
  value: string[];
  onChange: (value: string[]) => void;
  getLabel?: (option: string) => string;
}

export default function MultiFilter({ label, options, value, onChange, getLabel = (o) => o }: MultiFilterProps) {
  const { locale } = useLocale();
  // Options arrive as ids or slugs; the list is shown in alphabetical order of what the user actually reads.
  const sorted = useMemo(
    () => [...options].sort((a, b) => getLabel(a).localeCompare(getLabel(b), locale, { sensitivity: 'base', numeric: true })),
    [options, getLabel, locale]
  );
  return (
    <Autocomplete
      multiple
      options={sorted}
      value={value}
      getOptionLabel={getLabel}
      onChange={(_, v) => onChange(v)}
      renderTags={(v, getTagProps) =>
        v.map((option, i) => <Chip label={getLabel(option)} {...getTagProps({ index: i })} key={option} size="small" />)
      }
      renderInput={(params) => <TextField {...params} label={label} size="small" fullWidth />}
      size="small"
      fullWidth
    />
  );
}
