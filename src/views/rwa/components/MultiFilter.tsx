import Autocomplete from '@mui/material/Autocomplete';
import Chip from '@mui/material/Chip';
import TextField from '@mui/material/TextField';

interface MultiFilterProps {
  label: string;
  options: string[];
  value: string[];
  onChange: (value: string[]) => void;
  getLabel?: (option: string) => string;
}

export default function MultiFilter({ label, options, value, onChange, getLabel = (o) => o }: MultiFilterProps) {
  return (
    <Autocomplete
      multiple
      options={options}
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
