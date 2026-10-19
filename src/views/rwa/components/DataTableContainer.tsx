import { ReactNode, useEffect, useRef, useState } from 'react';
import { SxProps, Theme } from '@mui/material/styles';
import Paper from '@mui/material/Paper';
import TableContainer from '@mui/material/TableContainer';

const APP_BAR_FALLBACK = 80; // until the fixed app bar is measured: 48px toolbar + 16px padding on each side

// Bottom edge of the fixed app bar: the header must stick right under it, and its height depends on width and content.
function appBarBottom(): number {
  const bar = document.querySelector<HTMLElement>('header.MuiAppBar-positionFixed');
  return bar ? Math.round(bar.getBoundingClientRect().bottom) : APP_BAR_FALLBACK;
}

interface Props {
  sx?: SxProps<Theme>;
  children: ReactNode;
}

// Table on a Paper whose header stays under the app bar while the page scrolls.
// While the table fits its card, CSS `position: sticky` on the header cells does the job.
// A wider table scrolls sideways inside the card, and a scroll container cannot host a page-level sticky header,
// so then the header is shifted with a transform on every scroll frame instead.
export default function DataTableContainer({ sx = [], children }: Props) {
  const ref = useRef<HTMLDivElement>(null);
  const [fits, setFits] = useState(false); // prerendered HTML keeps the safe scrolling layout; measured on mount
  const [top, setTop] = useState(APP_BAR_FALLBACK);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const measure = () => {
      setFits(el.scrollWidth <= el.clientWidth + 1);
      setTop(appBarBottom());
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    if (el.firstElementChild) observer.observe(el.firstElementChild);
    const bar = document.querySelector('header.MuiAppBar-positionFixed');
    if (bar) observer.observe(bar);
    return () => observer.disconnect();
  }, []);

  // Fallback for a sideways-scrolling table: translate the header down by how far the card top has gone under the bar.
  useEffect(() => {
    const el = ref.current;
    const head = el?.querySelector<HTMLElement>('thead');
    if (fits || !el || !head) return;
    let frame = 0;
    const update = () => {
      frame = 0;
      const rect = el.getBoundingClientRect();
      const offset = Math.min(Math.max(top - rect.top, 0), Math.max(rect.height - head.offsetHeight, 0));
      head.style.transform = offset > 0 ? `translateY(${offset}px)` : '';
      head.classList.toggle('is-floating', offset > 0);
    };
    const schedule = () => {
      if (!frame) frame = requestAnimationFrame(update);
    };
    update();
    window.addEventListener('scroll', schedule, { passive: true });
    window.addEventListener('resize', schedule);
    return () => {
      window.removeEventListener('scroll', schedule);
      window.removeEventListener('resize', schedule);
      if (frame) cancelAnimationFrame(frame);
      head.style.transform = '';
      head.classList.remove('is-floating');
    };
  }, [fits, top]);

  const headCells = '& .MuiTableHead-root .MuiTableCell-root';
  const hairline = (theme: Theme) => `inset 0 -1px 0 ${theme.palette.divider}`; // keeps a line under the header while rows pass beneath

  return (
    <TableContainer
      ref={ref}
      component={Paper}
      sx={[
        // the containing block of the visually hidden hints inside the cells: their overflow then stays inside this
        // scrolling box instead of widening the page (a phone's layout viewport would follow it)
        { position: 'relative' },
        fits
          ? {
              overflow: 'visible',
              [headCells]: { position: 'sticky', top, zIndex: 2, backgroundColor: 'background.paper', boxShadow: hairline }
            }
          : {
              '& .MuiTableHead-root': { position: 'relative', zIndex: 2 },
              '& .MuiTableHead-root.is-floating .MuiTableCell-root': { backgroundColor: 'background.paper', boxShadow: hairline }
            },
        ...(Array.isArray(sx) ? sx : [sx])
      ]}
    >
      {children}
    </TableContainer>
  );
}
