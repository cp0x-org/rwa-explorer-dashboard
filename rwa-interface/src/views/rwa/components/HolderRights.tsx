import { Fragment, useEffect, useRef } from 'react';
import { useLocation } from 'react-router-dom';
import Box from '@mui/material/Box';
import Link from '@mui/material/Link';
import Typography from '@mui/material/Typography';

import type { RwaIndex } from 'api/rwa';
import useHydrated from 'hooks/useHydrated';
import useExpiry from 'hooks/useExpiry';
import { LocaleLink, MessageId, useFormat, useLocale, useT } from 'i18n';
import type { Issuer, RightsEvidence, RightsFact, RightsProfile, Token } from 'types/rwa';
import { safeUrl } from 'utils/links';
import { countryName, factState, placesOf, reviewIsStale, reviewRecheckDelay, SUMMARY_FACTS } from 'utils/rights';

// The same value renderer is used in the full profile, comparison and token summary.
export function RightsFactValue({ factKey, fact, compact = false }: { factKey: string; fact: RightsFact | undefined; compact?: boolean }) {
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
      <span>{value}</span>
      {compact && note && (
        <Typography variant="body2" sx={{ marginTop: 0.5 }}>
          {t('rights.conditions')}
        </Typography>
      )}
      {state === 'value' && places.length > 0 && (
        <Box sx={{ marginTop: 0.5 }}>
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
        <Typography variant="body2" sx={{ marginTop: 0.5 }}>
          {note}
        </Typography>
      )}
    </>
  );
}

export function RightsReview({ index, profile }: { index: RwaIndex; profile: RightsProfile }) {
  const t = useT();
  const f = useFormat();
  const now = useExpiry(profile.reviewed_at, index.data.summary.rights_review_valid_days, reviewRecheckDelay);
  const stale = profile.stale || (now && reviewIsStale(profile.reviewed_at, index.data.summary.rights_review_valid_days, now));
  return (
    <Typography component="p" variant="body2" sx={{ marginY: 1 }}>
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

function Sources({ evidence = [], showEmpty = false }: { evidence: RightsEvidence[] | undefined; showEmpty?: boolean }) {
  const t = useT();
  if (!evidence.length && !showEmpty) return null;
  return (
    <Box component="details" sx={{ marginTop: 1 }}>
      <summary>{t('rights.sources', { n: evidence.length })}</summary>
      {evidence.map((source, i) => (
        <Box key={`${source.url}:${i}`} sx={{ marginY: 2 }}>
          <Box
            component="blockquote"
            sx={{ margin: 0, marginBottom: 1, paddingLeft: 2, borderLeft: 2, borderColor: 'divider', whiteSpace: 'pre-wrap' }}
          >
            {source.quote}
          </Box>
          <DocumentLink document={source} />
        </Box>
      ))}
    </Box>
  );
}

function FactSources({ fact, profile }: { fact: RightsFact | undefined; profile: RightsProfile }) {
  const t = useT();
  if (factState(fact) !== 'unknown') return <Sources evidence={fact?.evidence} />;
  const checked = fact?.checked ?? [];
  return (
    <Box component="details" sx={{ marginTop: 1 }}>
      <summary>{t('rights.checked', { n: checked.length })}</summary>
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
      sx={{ display: 'inline-flex', minHeight: 24, alignItems: 'center' }}
    >
      {t('rights.suggest')}
    </Link>
  );
}

function Profile({ index, issuer, profile }: { index: RwaIndex; issuer: Issuer; profile: RightsProfile }) {
  const t = useT();
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
    <Box component="ul" sx={{ display: 'flex', gap: 2, flexWrap: 'wrap', paddingLeft: 2.5 }}>
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
        marginBottom: 4,
        scrollMarginTop: 100,
        overflowWrap: 'anywhere',
        '& a': { display: 'inline-flex', minHeight: 24, alignItems: 'center' },
        '& summary': { minHeight: 24, cursor: 'pointer' }
      }}
    >
      <Typography variant="h3" component="h2" gutterBottom>
        {t('rights.title')} · {profile.product}
      </Typography>
      <Box>
        {tokens.length > 12 ? (
          <details>
            <summary>{appliesTo}</summary>
            {tokenLinks}
          </details>
        ) : (
          <>
            <Typography>{appliesTo}</Typography>
            {tokenLinks}
          </>
        )}
      </Box>
      <Box>
        <Typography>{t(profile.scope_basis === 'programme_documents' ? 'rights.scope.programme' : 'rights.scope.per_token')}</Typography>
        <Sources evidence={profile.scope_evidence} />
      </Box>
      <Box id={`rights-issuer-${profile.id}`} sx={{ marginTop: 1 }}>
        <Typography>
          {t('rights.legalIssuer')}: {legal.name}
          {legal.country && ` · ${countryName(legal.country, locale)}`}
          {legal.zone && ` · ${legal.zone}`}
        </Typography>
        {legalNote && <Typography variant="body2">{legalNote}</Typography>}
        <Sources evidence={legal.evidence} showEmpty />
      </Box>
      <RightsReview index={index} profile={profile} />
      <Box component="dl" sx={{ margin: 0 }}>
        {SUMMARY_FACTS.map((key) => (
          <Fragment key={key}>
            <Typography component="dt" sx={{ fontWeight: 600, marginTop: 1.5 }}>
              {t(`rights.fact.${key}`)}
            </Typography>
            <Box component="dd" sx={{ margin: 0, marginTop: 0.5 }}>
              <RightsFactValue factKey={key} fact={profile.facts[key]} compact />
            </Box>
          </Fragment>
        ))}
      </Box>
      <Box component="details" ref={fullRef} id={`rights-full-${profile.id}`} sx={{ marginY: 3 }}>
        <summary>{t('rights.full.show', { n: schemaFacts.length })}</summary>
        {groups.map((group) => (
          <Box key={group} sx={{ marginY: 3 }}>
            <Typography variant="h4" component="h3" gutterBottom>
              {t(`rights.group.${group}` as MessageId)}
            </Typography>
            <Box component="dl" sx={{ margin: 0 }}>
              {schemaFacts
                .filter((fact) => fact.group === group)
                .map(({ key }) => (
                  <Fragment key={key}>
                    <Typography component="dt" sx={{ fontWeight: 600, paddingTop: 1.5 }}>
                      {t(`rights.fact.${key}` as MessageId)}
                    </Typography>
                    <Box component="dd" sx={{ margin: 0, marginTop: 0.5, paddingBottom: 1.5, borderBottom: 1, borderColor: 'divider' }}>
                      <RightsFactValue factKey={key} fact={profile.facts[key]} />
                      <FactSources fact={profile.facts[key]} profile={profile} />
                    </Box>
                  </Fragment>
                ))}
            </Box>
          </Box>
        ))}
        <Typography variant="h4" component="h3">
          {t('rights.documents')}
        </Typography>
        <ul>
          {profile.documents.map((document) => (
            <li key={document.url}>
              <DocumentLink document={document} />
            </li>
          ))}
        </ul>
      </Box>
      <Typography variant="body2" sx={{ marginY: 2 }}>
        {t('rights.disclaimer')}
      </Typography>
      <SuggestCorrection />
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
        <section>
          <Typography variant="h3" component="h2" gutterBottom>
            {t('rights.title')}
          </Typography>
          <Typography sx={{ marginBottom: 1 }}>{t('rights.notReviewed')}</Typography>
          <SuggestCorrection />
        </section>
      )}
    </Box>
  );
}
