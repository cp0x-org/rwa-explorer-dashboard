import { Fragment, ReactNode, useEffect, useRef } from 'react';
import { useLocation } from 'react-router-dom';
import { alpha, useTheme } from '@mui/material/styles';
import type { SxProps, Theme } from '@mui/material/styles';
import Box from '@mui/material/Box';
import Link from '@mui/material/Link';
import Typography from '@mui/material/Typography';
import {
  IconAlertOctagon,
  IconAlertTriangle,
  IconBuildingBank,
  IconCalendar,
  IconCalendarExclamation,
  IconChevronRight,
  IconCircleCheck,
  IconCircleDot,
  IconCircleX,
  IconCoin,
  IconFileDescription,
  IconFileText,
  IconFiles,
  IconHelpCircle,
  IconInfoCircle,
  IconLock,
  IconPencil,
  IconScale,
  IconStack2,
  IconUsers
} from '@tabler/icons-react';

import type { RwaIndex } from 'api/rwa';
import useHydrated from 'hooks/useHydrated';
import useExpiry from 'hooks/useExpiry';
import { LocaleLink, MessageId, useFormat, useLocale, useT } from 'i18n';
import type { Issuer, RightsEvidence, RightsFact, RightsProfile, Token } from 'types/rwa';
import { safeUrl } from 'utils/links';
import { countryName, factState, factTone, FactTone, placesOf, reviewIsStale, reviewRecheckDelay, SUMMARY_FACTS } from 'utils/rights';

type TablerIcon = typeof IconCircleCheck;

const TONE_ICON: Record<FactTone, TablerIcon> = {
  positive: IconCircleCheck,
  caution: IconAlertTriangle,
  negative: IconCircleX,
  neutral: IconCircleDot,
  unknown: IconHelpCircle
};

const GROUP_ICON: Record<string, TablerIcon> = {
  what: IconFileDescription,
  who: IconUsers,
  issuer_powers: IconLock,
  loss: IconAlertOctagon,
  shareholder: IconCoin,
  other: IconScale
};

// Text colour of a tone; the badge background and border are tints of it. Both themes keep small text above 4.5:1.
export function toneColor(theme: Theme, tone: FactTone): string {
  const dark = theme.palette.mode === 'dark';
  switch (tone) {
    case 'positive':
      return dark ? '#69f0ae' : '#1b5e20';
    case 'caution':
      return dark ? '#ffd54f' : '#7a4b00';
    case 'negative':
      return dark ? '#ff8a80' : '#b71c1c';
    case 'neutral':
      return theme.palette.text.primary;
    default:
      return theme.palette.text.secondary;
  }
}

// plain: the icon and the coloured text without the box, for dense tables
function FactBadge({ tone, plain = false, children }: { tone: FactTone; plain?: boolean; children: ReactNode }) {
  const theme = useTheme();
  const color = toneColor(theme, tone);
  const Icon = TONE_ICON[tone];
  if (plain)
    return (
      <Box component="span" data-tone={tone} sx={{ display: 'inline-flex', alignItems: 'flex-start', gap: 0.5, color, fontWeight: 500 }}>
        <Icon size={15} stroke={2} aria-hidden style={{ flexShrink: 0, marginTop: 3 }} />
        <span>{children}</span>
      </Box>
    );
  return (
    <Box
      component="span"
      data-tone={tone}
      sx={{
        display: 'inline-flex',
        alignItems: 'flex-start',
        gap: 0.75,
        paddingX: 1.25,
        paddingY: 0.5,
        borderRadius: '8px',
        border: '1px solid',
        borderColor: alpha(color, tone === 'neutral' || tone === 'unknown' ? 0.25 : 0.4),
        backgroundColor: alpha(color, tone === 'neutral' || tone === 'unknown' ? 0.05 : 0.12),
        color,
        fontWeight: 500,
        lineHeight: 1.45,
        maxWidth: '100%'
      }}
    >
      <Icon size={16} stroke={2} aria-hidden style={{ flexShrink: 0, marginTop: 2 }} />
      <span>{children}</span>
    </Box>
  );
}

// What the colours of the badges mean; shown next to every set of them.
export function ToneLegend({ sx }: { sx?: SxProps<Theme> }) {
  const t = useT();
  const theme = useTheme();
  const tones: FactTone[] = ['positive', 'caution', 'negative', 'unknown'];
  return (
    <Box component="ul" sx={{ display: 'flex', flexWrap: 'wrap', gap: { xs: 1, sm: 2 }, listStyle: 'none', padding: 0, margin: 0, ...sx }}>
      {tones.map((tone) => {
        const Icon = TONE_ICON[tone];
        return (
          <Typography
            key={tone}
            component="li"
            variant="body2"
            sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5, color: 'text.secondary' }}
          >
            <Icon size={15} stroke={2} aria-hidden color={toneColor(theme, tone)} />
            {t(`rights.tone.${tone}` as MessageId)}
          </Typography>
        );
      })}
    </Box>
  );
}

// The same value renderer is used in the full profile, comparison and token summary.
export function RightsFactValue({
  factKey,
  fact,
  compact = false,
  plain = false
}: {
  factKey: string;
  fact: RightsFact | undefined;
  compact?: boolean;
  plain?: boolean;
}) {
  const t = useT();
  const { locale } = useLocale();
  const state = factState(fact);
  const { kind, entries } = placesOf(factKey, fact, locale);
  const places = [...(kind === 'listed' && fact?.us_persons ? [t('rights.restricted.us')] : []), ...entries];
  const placesText = (items: string[]) => t(kind === 'only' ? 'rights.places' : 'rights.places.list', { places: items.join(', ') });
  const note = fact?.note?.[locale === 'zh-CN' ? 'zh' : 'en'];
  const value =
    state === 'missing'
      ? t('rights.notReviewed.short')
      : state === 'unknown'
        ? t(note ? 'rights.value.unresolved' : 'rights.value.unknown')
        : t(`rights.value.${factKey}.${fact!.value}` as MessageId, {
            who: fact?.who ? t(`rights.who.${fact.who}` as MessageId) : undefined
          });

  return (
    <>
      <FactBadge tone={factTone(factKey, fact)} plain={plain}>
        {value}
      </FactBadge>
      {compact && note && (
        <Typography variant="body2" sx={{ marginTop: 0.75, display: 'flex', alignItems: 'center', gap: 0.5, color: 'text.secondary' }}>
          <IconInfoCircle size={14} stroke={2} aria-hidden style={{ flexShrink: 0 }} />
          {t('rights.conditions')}
        </Typography>
      )}
      {state === 'value' && places.length > 0 && (
        <Box sx={{ marginTop: 0.75, fontWeight: 500 }}>
          {compact && kind !== 'jurisdiction' && places.length > 3
            ? t(kind === 'only' ? 'rights.places.count.only' : 'rights.places.count', { n: places.length })
            : placesText(kind === 'jurisdiction' ? places : places.slice(0, compact ? 3 : 6))}
          {!compact && kind !== 'jurisdiction' && places.length > 6 && (
            <details>
              <summary>{t('rights.places.more', { n: places.length - 6 })}</summary>
              {placesText(places)}
            </details>
          )}
        </Box>
      )}
      {!compact && note && (
        <Typography variant="body2" sx={{ marginTop: 1, color: 'text.secondary', lineHeight: 1.6 }}>
          {note}
        </Typography>
      )}
    </>
  );
}

export function RightsReview({ index, profile }: { index: RwaIndex; profile: RightsProfile }) {
  const t = useT();
  const f = useFormat();
  const theme = useTheme();
  const now = useExpiry(profile.reviewed_at, index.data.summary.rights_review_valid_days, reviewRecheckDelay);
  const stale = profile.stale || (now && reviewIsStale(profile.reviewed_at, index.data.summary.rights_review_valid_days, now));
  const Icon = stale ? IconCalendarExclamation : IconCalendar;
  return (
    <Typography
      component="p"
      variant="body2"
      sx={{
        marginY: 1,
        display: 'flex',
        alignItems: 'center',
        gap: 0.75,
        color: stale ? toneColor(theme, 'caution') : 'text.secondary'
      }}
    >
      <Icon size={16} stroke={2} aria-hidden style={{ flexShrink: 0 }} />
      {t(stale ? 'rights.stale' : 'rights.reviewedAt', { date: f.day(profile.reviewed_at) })}
    </Typography>
  );
}

function DocumentLink({ document }: { document: { url: string; title: string; doc_date?: string } }) {
  const t = useT();
  const href = safeUrl(document.url);
  const label = document.doc_date ? t('rights.document.dated', { title: document.title, date: document.doc_date }) : document.title;
  return href ? (
    <Link
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      underline="always"
      sx={{ overflowWrap: 'anywhere', display: 'inline-flex', minHeight: 24, alignItems: 'center' }}
    >
      {label}
    </Link>
  ) : (
    <span>{label}</span>
  );
}

// A disclosure whose summary is a small link-like control with a chevron that turns when it opens.
const disclosureSx = (theme: Theme) => ({
  marginTop: 1,
  '& > summary': {
    listStyle: 'none',
    display: 'inline-flex',
    alignItems: 'center',
    gap: 0.5,
    minHeight: 24,
    cursor: 'pointer',
    color: theme.palette.primary.main,
    fontSize: '0.8125rem',
    fontWeight: 500,
    '&::-webkit-details-marker': { display: 'none' },
    '& > svg': { transition: 'transform 0.15s' }
  },
  '&[open] > summary > svg': { transform: 'rotate(90deg)' }
});

const Chevron = () => <IconChevronRight size={14} stroke={2.5} aria-hidden />;

function Quote({ children }: { children: ReactNode }) {
  const theme = useTheme();
  return (
    <Box
      component="blockquote"
      sx={{
        margin: 0,
        marginBottom: 1,
        paddingX: 2,
        paddingY: 1.25,
        borderLeft: 3,
        borderColor: alpha(theme.palette.primary.main, 0.6),
        borderRadius: '0 8px 8px 0',
        backgroundColor: alpha(theme.palette.text.primary, 0.04),
        whiteSpace: 'pre-wrap',
        fontSize: '0.875rem',
        lineHeight: 1.6
      }}
    >
      {children}
    </Box>
  );
}

function Sources({ evidence = [], showEmpty = false }: { evidence: RightsEvidence[] | undefined; showEmpty?: boolean }) {
  const t = useT();
  const theme = useTheme();
  if (!evidence.length && !showEmpty) return null;
  return (
    <Box component="details" sx={disclosureSx(theme)}>
      <summary>
        <Chevron />
        {t('rights.sources', { n: evidence.length })}
      </summary>
      {evidence.map((source, i) => (
        <Box key={`${source.url}:${i}`} sx={{ marginY: 1.5 }}>
          <Quote>{source.quote}</Quote>
          <DocumentLink document={source} />
        </Box>
      ))}
    </Box>
  );
}

function FactSources({ fact, profile }: { fact: RightsFact | undefined; profile: RightsProfile }) {
  const t = useT();
  const theme = useTheme();
  if (factState(fact) !== 'unknown') return <Sources evidence={fact?.evidence} />;
  const checked = fact?.checked ?? [];
  return (
    <Box component="details" sx={disclosureSx(theme)}>
      <summary>
        <Chevron />
        {t('rights.checked', { n: checked.length })}
      </summary>
      <ul>
        {checked.map((url) => (
          <li key={url}>
            <DocumentLink document={profile.documents.find((document) => document.url === url) ?? { url, title: url }} />
          </li>
        ))}
      </ul>
    </Box>
  );
}

export function SuggestCorrection() {
  const t = useT();
  return (
    <Link
      href={safeUrl('https://github.com/cp0x-org/rwa-explorer-dashboard/issues/new')}
      target="_blank"
      rel="noopener noreferrer"
      underline="always"
      sx={{ display: 'inline-flex', gap: 0.5, minHeight: 24, alignItems: 'center' }}
    >
      <IconPencil size={15} stroke={2} aria-hidden />
      {t('rights.suggest')}
    </Link>
  );
}

// The short facts as tiles: a coloured top edge repeats the tone of the value. Used on the issuer and token pages.
export function KeyFacts({ profile }: { profile: RightsProfile }) {
  const t = useT();
  const theme = useTheme();
  return (
    <Box
      component="dl"
      sx={{
        margin: 0,
        display: 'grid',
        gridTemplateColumns: { xs: '1fr', sm: 'repeat(2, 1fr)', lg: 'repeat(4, 1fr)' },
        gap: 1.5
      }}
    >
      {SUMMARY_FACTS.map((key) => (
        <Box
          key={key}
          sx={{
            padding: 2,
            borderRadius: '10px',
            border: '1px solid',
            borderColor: 'divider',
            borderTop: '3px solid',
            borderTopColor: toneColor(theme, factTone(key, profile.facts[key])),
            backgroundColor: alpha(theme.palette.text.primary, 0.02)
          }}
        >
          <Typography component="dt" variant="body2" sx={{ color: 'text.secondary', fontWeight: 500, marginBottom: 1 }}>
            {t(`rights.fact.${key}`)}
          </Typography>
          <Box component="dd" sx={{ margin: 0 }}>
            <RightsFactValue factKey={key} fact={profile.facts[key]} compact />
          </Box>
        </Box>
      ))}
    </Box>
  );
}

// A labelled box of the profile header: who stands behind the token, what the review covers.
function InfoBox({ icon: Icon, label, children, id }: { icon: TablerIcon; label: string; children: ReactNode; id?: string }) {
  return (
    <Box id={id} sx={{ padding: 2, borderRadius: '10px', border: '1px solid', borderColor: 'divider', minWidth: 0 }}>
      <Typography
        variant="body2"
        sx={{ display: 'flex', alignItems: 'center', gap: 0.75, color: 'text.secondary', fontWeight: 500, marginBottom: 0.75 }}
      >
        <Icon size={16} stroke={2} aria-hidden />
        {label}
      </Typography>
      {children}
    </Box>
  );
}

const cardSx = {
  backgroundColor: 'background.default',
  borderRadius: '12px',
  padding: { xs: 2, sm: 3 }
};

function Profile({ index, issuer, profile }: { index: RwaIndex; issuer: Issuer; profile: RightsProfile }) {
  const t = useT();
  const theme = useTheme();
  const { locale } = useLocale();
  const hydrated = useHydrated();
  const { hash } = useLocation();
  const fullRef = useRef<HTMLDetailsElement>(null);
  useEffect(() => {
    if (!hydrated || hash !== `#rights-${profile.id}`) return;
    if (fullRef.current) fullRef.current.open = true;
    const frame = requestAnimationFrame(() => document.getElementById(`rights-${profile.id}`)?.scrollIntoView());
    return () => cancelAnimationFrame(frame);
  }, [hydrated, hash, profile.id]);
  const tokens = profile.token_ids.map((id) => index.tokenById.get(id)).filter((token): token is Token => !!token);
  const issuerTokenCount = index.data.tokens.filter((token) => token.issuer_id === issuer.id).length;
  const appliesTo = t('rights.appliesTo', { n: tokens.length, m: issuerTokenCount });
  const tokenLinks = (
    <Box
      component="ul"
      sx={{ display: 'flex', columnGap: 1.5, rowGap: 0.25, flexWrap: 'wrap', paddingLeft: 0, listStyle: 'none', marginY: 1 }}
    >
      {tokens.map((token) => (
        <li key={token.id}>
          <Link component={LocaleLink} to={`/tokens/${token.id}`} underline="always">
            {token.symbol ?? token.name ?? String(token.id)}
          </Link>
        </li>
      ))}
    </Box>
  );
  const legal = profile.legal_issuer;
  const legalNote = legal.note?.[locale === 'zh-CN' ? 'zh' : 'en'];
  const schemaFacts = index.data.rights_schema?.facts ?? [];
  const groups = [...new Set(schemaFacts.map((fact) => fact.group))];

  return (
    <Box
      component="section"
      id={`rights-${profile.id}`}
      sx={{
        ...cardSx,
        marginBottom: 3,
        scrollMarginTop: 100,
        overflowWrap: 'anywhere',
        '& a': { display: 'inline-flex', minHeight: 24, alignItems: 'center' },
        '& summary': { minHeight: 24, cursor: 'pointer' }
      }}
    >
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', flexWrap: 'wrap', columnGap: 2 }}>
        <Typography variant="h3" component="h2" sx={{ marginBottom: 0.5 }}>
          {t('rights.title')} · {profile.product}
        </Typography>
        <RightsReview index={index} profile={profile} />
      </Box>

      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: '1fr 1.4fr' }, gap: 1.5, marginY: 2 }}>
        <InfoBox icon={IconStack2} label={t('rights.scope')}>
          {tokens.length > 12 ? (
            <Box component="details" sx={disclosureSx(theme)}>
              <summary>
                <Chevron />
                {appliesTo}
              </summary>
              {tokenLinks}
            </Box>
          ) : (
            <>
              <Typography sx={{ fontWeight: 500 }}>{appliesTo}</Typography>
              {tokenLinks}
            </>
          )}
          <Typography variant="body2" sx={{ marginTop: 1, color: 'text.secondary' }}>
            {t(profile.scope_basis === 'programme_documents' ? 'rights.scope.programme' : 'rights.scope.per_token')}
          </Typography>
          <Sources evidence={profile.scope_evidence} />
        </InfoBox>
        <InfoBox icon={IconBuildingBank} label={t('rights.legalIssuer')} id={`rights-issuer-${profile.id}`}>
          <Typography sx={{ fontWeight: 600 }}>
            {legal.name}
            {legal.country && (
              <Box component="span" sx={{ fontWeight: 400, color: 'text.secondary' }}>
                {` · ${countryName(legal.country, locale)}`}
              </Box>
            )}
            {legal.zone && (
              <Box component="span" sx={{ fontWeight: 400, color: 'text.secondary' }}>
                {` · ${legal.zone}`}
              </Box>
            )}
          </Typography>
          {legalNote && (
            <Typography variant="body2" sx={{ marginTop: 0.75, color: 'text.secondary' }}>
              {legalNote}
            </Typography>
          )}
          <Sources evidence={legal.evidence} showEmpty />
        </InfoBox>
      </Box>

      <Typography variant="h4" component="h3" sx={{ marginBottom: 1.5 }}>
        {t('rights.keyFacts')}
      </Typography>
      <KeyFacts profile={profile} />
      <ToneLegend sx={{ marginTop: 1.5 }} />

      <Box
        component="details"
        ref={fullRef}
        id={`rights-full-${profile.id}`}
        sx={{
          marginTop: 3,
          '& > summary': {
            listStyle: 'none',
            display: 'inline-flex',
            alignItems: 'center',
            gap: 1,
            paddingX: 2,
            paddingY: 1,
            borderRadius: '8px',
            border: '1px solid',
            borderColor: alpha(theme.palette.primary.main, 0.5),
            color: theme.palette.primary.main,
            fontWeight: 500,
            '&::-webkit-details-marker': { display: 'none' },
            '&:hover': { backgroundColor: alpha(theme.palette.primary.main, 0.08) },
            '& > svg': { transition: 'transform 0.15s' }
          },
          '&[open] > summary > svg': { transform: 'rotate(90deg)' }
        }}
      >
        <summary>
          <IconChevronRight size={16} stroke={2.5} aria-hidden />
          {t('rights.full.show', { n: schemaFacts.length })}
        </summary>
        {groups.map((group) => {
          const GroupIcon = GROUP_ICON[group] ?? IconCircleDot;
          return (
            <Box key={group} sx={{ marginTop: 3 }}>
              <Typography variant="h4" component="h3" sx={{ display: 'flex', alignItems: 'center', gap: 1, marginBottom: 1 }}>
                <GroupIcon size={20} stroke={1.75} aria-hidden color={theme.palette.primary.main} />
                {t(`rights.group.${group}` as MessageId)}
              </Typography>
              <Box
                component="dl"
                sx={{
                  margin: 0,
                  paddingX: 2,
                  borderRadius: '10px',
                  border: '1px solid',
                  borderColor: 'divider',
                  display: 'grid',
                  gridTemplateColumns: { xs: '1fr', md: 'minmax(180px, 260px) 1fr' },
                  columnGap: 3,
                  '& > dt:first-of-type, & > dd:first-of-type': { borderTop: 0 }
                }}
              >
                {schemaFacts
                  .filter((fact) => fact.group === group)
                  .map(({ key }) => (
                    <Fragment key={key}>
                      <Typography
                        component="dt"
                        sx={{ fontWeight: 600, paddingTop: 2, paddingBottom: { md: 2 }, borderTop: 1, borderColor: 'divider' }}
                      >
                        {t(`rights.fact.${key}` as MessageId)}
                      </Typography>
                      <Box
                        component="dd"
                        sx={{
                          margin: 0,
                          paddingTop: { xs: 1, md: 2 },
                          paddingBottom: 2,
                          borderTop: { xs: 0, md: 1 },
                          borderColor: { md: 'divider' }
                        }}
                      >
                        <RightsFactValue factKey={key} fact={profile.facts[key]} />
                        <FactSources fact={profile.facts[key]} profile={profile} />
                      </Box>
                    </Fragment>
                  ))}
              </Box>
            </Box>
          );
        })}
        <Box sx={{ marginTop: 3 }}>
          <Typography variant="h4" component="h3" sx={{ display: 'flex', alignItems: 'center', gap: 1, marginBottom: 1 }}>
            <IconFiles size={20} stroke={1.75} aria-hidden color={theme.palette.primary.main} />
            {t('rights.documents')}
          </Typography>
          <Box
            component="ul"
            sx={{ listStyle: 'none', padding: 0, margin: 0, display: 'grid', gridTemplateColumns: { xs: '1fr', md: '1fr 1fr' }, gap: 1 }}
          >
            {profile.documents.map((document) => (
              <Box
                component="li"
                key={document.url}
                sx={{
                  display: 'flex',
                  alignItems: 'flex-start',
                  gap: 1,
                  padding: 1.25,
                  borderRadius: '8px',
                  border: '1px solid',
                  borderColor: 'divider'
                }}
              >
                <IconFileText size={18} stroke={1.75} aria-hidden style={{ flexShrink: 0, marginTop: 3 }} />
                <DocumentLink document={document} />
              </Box>
            ))}
          </Box>
        </Box>
      </Box>

      <Box
        sx={{
          marginTop: 3,
          paddingTop: 2,
          borderTop: 1,
          borderColor: 'divider',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'flex-start',
          flexWrap: 'wrap',
          gap: 2
        }}
      >
        <Typography variant="body2" sx={{ display: 'flex', gap: 0.75, color: 'text.secondary', maxWidth: 820 }}>
          <IconInfoCircle size={16} stroke={2} aria-hidden style={{ flexShrink: 0, marginTop: 2 }} />
          {t('rights.disclaimer')}
        </Typography>
        <SuggestCorrection />
      </Box>
    </Box>
  );
}

export default function HolderRights({ index, issuer }: { index: RwaIndex; issuer: Issuer }) {
  const t = useT();
  const profiles = (issuer.rights_profile_ids ?? [])
    .map((id) => index.rightsById.get(id))
    .filter((profile): profile is RightsProfile => !!profile && profile.issuer_id === issuer.id);
  return (
    <Box id="rights" sx={{ marginY: 4, scrollMarginTop: 100 }}>
      {profiles.length ? (
        profiles.map((profile) => <Profile key={profile.id} index={index} issuer={issuer} profile={profile} />)
      ) : (
        <Box component="section" sx={cardSx}>
          <Typography variant="h3" component="h2" gutterBottom>
            {t('rights.title')}
          </Typography>
          <Typography sx={{ marginBottom: 1, display: 'flex', alignItems: 'center', gap: 0.75, color: 'text.secondary' }}>
            <IconHelpCircle size={18} stroke={2} aria-hidden style={{ flexShrink: 0 }} />
            {t('rights.notReviewed')}
          </Typography>
          <SuggestCorrection />
        </Box>
      )}
    </Box>
  );
}
