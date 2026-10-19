import { visuallyHidden } from 'ui-component/VisuallyHidden';

// Accessible name of a data table: a real <caption>, visually hidden (the page heading above the table says the same).
export default function TableCaption({ children }: { children: string }) {
  return <caption style={visuallyHidden}>{children}</caption>;
}
