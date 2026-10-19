// English, the source of truth: every id used in the app is a key here, and zh.ts must define the same keys.
// Values are ICU messages (react-intl): {name} interpolation, {n, plural, one {...} other {...}}.
// Never put tickers, symbols, addresses, protocol or network names into messages: they are data, not UI text.

const en = {
  // ---------- site ----------
  'site.name': 'Tokenized Stocks in DeFi by cp0x',
  'site.brand': 'cp0x',
  'site.noscript': 'JavaScript is off: this is a static snapshot of the page. Sorting, filters and paging need JavaScript.',
  'site.skipToContent': 'Skip to content',

  // ---------- header / navigation ----------
  'nav.sections': 'Sections',
  'nav.assets': 'Underlying assets',
  'nav.assets.short': 'Underlying',
  'nav.tokenized': 'Tokenized assets',
  'nav.tokenized.short': 'Tokenized',
  'nav.issuers': 'Issuers',
  'nav.issuers.short': 'Issuers',
  'nav.positions': 'Positions',
  'nav.positions.short': 'Positions',
  'nav.summary': 'Summary',
  'nav.summary.short': 'Summary',
  'header.home': 'Home',
  'header.pi': 'Permissionless Interfaces',
  'header.referrals': 'cp0x Referrals',
  'header.siteMenu': 'Site menu',
  'header.openMenu': 'Open menu',
  'header.closeMenu': 'Close menu',
  'header.menu': 'Menu',
  'header.logo': 'cp0x — Tokenized Stocks in DeFi, home',
  'lang.label': 'Language',
  'lang.switchTo': 'Switch to {language}',
  'footer.logo': 'cp0x',
  'footer.telegram': 'cp0x on Telegram',
  'footer.x': 'cp0x on X (Twitter)',
  'footer.github': 'cp0x on GitHub',
  'footer.donations': 'Donation addresses',

  // ---------- data loading / not found ----------
  'data.loading': 'Loading data…',
  'data.unavailable': 'Data unavailable',
  'data.exportMissing': 'Export not found.',
  'data.publishHint': 'Publish it with `uv run rwa-pipeline build` in rwa-data-pipeline.',
  'notFound.asset': 'Asset {ticker} not found in the current data',
  'notFound.token': 'Token {id} not found in the current data',
  'notFound.issuer': 'Issuer not found in the current data',
  'notFound.page': 'Page not found',
  'notFound.pageText': 'There is no page at this address. It may have been removed from the current data set, or the link is wrong.',
  'notFound.goHome': 'Go to the underlying assets',

  // ---------- method note ----------
  'method.collected': 'Data collected',
  'method.sources': 'Sources: CoinMarketCap, Portals',
  'method.coverage': 'Coverage: search',
  'method.how': 'How the rating works',
  'method.text':
    'Rating = number of eligible DeFi positions that contain the token, confirmed by its contract address in the position ' +
    'composition (directly or via LP → vault, up to {depth} levels). Eligible: real use-case (LP, lending supply, vault, staking, ' +
    'yield market), liquidity ≥ {liquidity}, source data ≤ {hours}h old at collection. APY is the reported APY of one position, ' +
    'not an expected return. Only positions indexed by Portals and found by search are covered; unsupported networks mean unknown ' +
    'usage, not zero.',

  // ---------- shared labels ----------
  'legend.title': 'Legend',
  'common.unknownIssuer': 'Unknown issuer',
  'common.otherIssuers': 'Other issuers',
  'common.wrapper': 'Wrapper',
  'common.details': 'Details',
  'common.showAll': 'Show all {n} assets',
  'common.showTop': 'Show top {n}',
  'common.all': 'All',
  'common.top': 'Top {n}',
  'common.any': 'Any (≥ 1)',
  'common.atLeast': '≥ {n}',
  'common.of': '{part} of {whole}',
  'common.rankOf': '#{rank} of {count}',
  'common.breadcrumb': 'Breadcrumb',

  // stat cards
  'stat.underlyings': 'Underlying assets',
  'stat.tokens': 'Tokenized assets',
  'stat.tokens.hint': 'Derivatives and other share classes excluded',
  'stat.issuers': 'Issuers',
  'stat.eligible': 'Eligible positions',
  'stat.eligible.hintRaw': '{n} confirmed by address before liquidity/freshness filters',
  'stat.eligible.hintDistinct': 'Distinct positions across tokenized assets',
  'stat.eligible.hintDistinctAll': 'Distinct positions across all tokenized assets',
  'stat.protocols': 'Protocols',
  'stat.networks': 'Networks',
  'stat.deployments': 'Deployments in Portals',
  'stat.deployments.hint': 'Contract deployments indexed by Portals out of all CMC deployments',
  'stat.rawConfirmed': 'Raw confirmed',
  'stat.rawConfirmed.hint': 'Confirmed by address, before filters',
  'stat.tokenizedMcap': 'Tokenized MCap (CMC)',
  'stat.avgPrice': 'Avg tokenized price (CMC)',
  'stat.usefulness': 'Usefulness score',
  'stat.usefulness.hint': 'Number of eligible DeFi positions',
  'stat.directIndirect': 'Direct / Indirect',
  'stat.directIndirect.hint': 'Indirect: e.g. vault holding an LP with the token',
  'stat.maxReportedApy': 'Max reported APY',
  'stat.maxReportedApy.hint': 'Highest reported APY of one eligible position',
  'stat.medianReportedApy': 'Median reported APY',
  'stat.marketCap': 'Market cap (CMC)',
  'stat.score': 'Score',
  'stat.score.hint': 'Unique eligible positions',
  'stat.relative': 'Relative basket score ({n})',
  'stat.underlyingTokenized': 'Underlying / tokenized assets',
  'stat.deploymentsPortals': 'Deployments on Portals networks',
  'stat.directPools': 'Direct pools liquidity',
  'stat.directPools.hint': 'Liquidity of tracked distinct direct pools. Not issuer TVL or capital',

  // table columns
  'col.asset': 'Asset',
  'col.tokenized': 'Tokenized assets',
  'col.tokenized.hint': 'Tokenized assets of the stock or ETF tracked by CoinMarketCap',
  'col.issuers': 'Issuers',
  'col.eligible': 'Eligible',
  'col.eligible.hint': 'Distinct eligible DeFi positions across all tokenized assets',
  'col.raw': 'Raw',
  'col.raw.hint': 'Distinct positions confirmed by address, before filters',
  'col.protocols': 'Protocols',
  'col.tokenizedMcap': 'Tokenized MCap',
  'col.tokenizedMcap.hint': 'CoinMarketCap tokenized market cap',
  'col.updated': 'Updated',
  'col.symbol': 'Symbol',
  'col.name': 'Name',
  'col.cmcIssuerLabel': 'CMC issuer label',
  'col.reason': 'Reason',
  'col.underlying': 'Underlying',
  'col.token': 'Token',
  'col.issuer': 'Issuer',
  'col.networks': 'Networks',
  'col.score': 'Score',
  'col.score.hint': 'Eligible positions; ties: protocols, networks, direct positions',
  'col.score.hintUnique': 'Unique eligible positions',
  'col.directIndirect': 'Direct / Indirect',
  'col.apyMaxMedian': 'APY max / median',
  'col.apyMaxMedian.hint': 'Reported APY over eligible positions: highest and median',
  'col.mcapCmc': 'MCap (CMC)',
  'col.rank': '#',
  'col.relative': 'Relative',
  'col.assets': 'Assets',
  'col.assets.hint': 'Underlying assets / the issuer’s tokenized assets of them',
  'col.deployments': 'Deployments',
  'col.deployments.hint': 'Deployments on networks covered by Portals / all CMC deployments',
  'col.yield': 'Yield',
  'col.yield.hint': 'Eligible positions with positive reported APY',
  'col.poolsLiquidity': 'Pools liquidity',
  'col.poolsLiquidity.hint': 'Liquidity of tracked distinct direct pools. Not issuer TVL',
  'col.rankInAsset': 'Rank in asset',
  'col.liquidity': 'Liquidity',
  'col.liquidity.hintBest': 'Liquidity of the best position',
  'col.maxApy': 'Max APY',
  'col.maxApy.hint': 'Highest reported APY over eligible positions. APY of one position, not an expected return',
  'col.updated.hintBest': 'Source timestamp of the best position',
  'col.position': 'Position',
  'col.protocol': 'Protocol',
  'col.network': 'Network',
  'col.category': 'Category',
  'col.link': 'Link',
  'col.reportedApy': 'Reported APY',
  'col.reportedApy.hint': 'Highest reported APY first. APY of one position, not an expected return',
  'col.status': 'Status',
  'col.networkCmc': 'Network (CMC)',
  'col.portalsNetwork': 'Portals network',
  'col.address': 'Address',
  'col.portalsSymbol': 'Portals symbol',
  'col.decimals': 'Decimals',
  'col.share': 'Share',
  'col.eligiblePositions': 'Eligible positions',
  'col.tokensInDefi': 'Tokenized assets in DeFi',
  'col.positionsWithApy': 'Positions with APY',
  'col.avgApy': 'Avg APY',
  'sort.hint': 'Click to sort by {label}',

  // table captions (visually hidden)
  'table.assets': 'Underlying assets ranked by DeFi usage of their tokenized assets',
  'table.excluded': 'Tokens excluded from the ranking',
  'table.tokenized': 'Tokenized assets ranked by eligible DeFi positions',
  'table.deployments': 'Contract deployments of the token',
  'table.breakdown': 'Eligible and raw positions by {title}',
  'table.positions': 'DeFi positions',
  'table.issuers': 'Issuers ranked by unique eligible DeFi positions',
  'table.issuerTokens': 'Tokenized assets of the issuer in the basket',
  'table.issuerSummary': 'Issuers: share of eligible positions and yield',
  'table.tokenSummary': 'Tokenized assets of the underlying: positions and yield',

  // filters, pager
  'filter.tickerOrName': 'Filter by ticker or name',
  'filter.underlying': 'Filter by underlying',
  'filter.issuer': 'Filter by issuer',
  'filter.network': 'Filter by network',
  'filter.symbolOrName': 'Filter by symbol or name',
  'filter.category': 'Filter by category',
  'filter.protocol': 'Filter by protocol',
  'filter.name': 'Filter by name',
  'filter.status': 'Status',
  'filter.link': 'Link',
  'filter.show': 'Filters',
  'filter.hide': 'Hide filters',
  'filter.active': '{n} active',
  'filter.status.allConfirmed': 'All confirmed',
  'filter.status.eligibleOnly': 'Eligible only',
  'filter.status.eligible': 'Eligible',
  'filter.status.confirmedRaw': 'All confirmed (raw)',
  'filter.status.inclUnconfirmed': 'Incl. unconfirmed symbol matches',
  'filter.link.all': 'Direct and indirect',
  'filter.link.direct': 'Direct',
  'filter.link.indirect': 'Indirect',
  'pager.rows': 'Rows',
  'pager.label': 'Table pages',
  'pager.page': 'Go to page {page}',
  'pager.current': 'Page {page}',
  'pager.next': 'Go to next page',
  'pager.previous': 'Go to previous page',
  'pager.first': 'Go to first page',
  'pager.last': 'Go to last page',

  // ---------- assets page ----------
  'assets.title': 'Underlying Assets',
  'assets.intro':
    'Stocks and ETFs with tokenized versions on-chain, ranked by how much their tokenized assets are actually used in DeFi: ' +
    'eligible positions (DEX pools, lending markets, vaults, yield markets) confirmed by contract address. Open an asset to ' +
    'compare its tokenized versions and issuers.',
  'assets.showing': '{n, plural, one {Showing # asset} other {Showing # assets}}',
  'assets.filteredBy': ' (filtered by "{query}")',
  'legend.assets.asset': 'Ticker and name of the underlying stock or ETF. Open a row to compare its tokenized assets.',
  'legend.assets.tokenized':
    'On-chain tokens of the stock or ETF tracked by CoinMarketCap (one per issuer product). Derivatives and tokens of another share class are excluded.',
  'legend.assets.issuers': 'Distinct issuers of these tokenized assets, as labeled by CoinMarketCap.',
  'legend.assets.eligible':
    'Distinct DeFi positions that contain any of its tokenized assets and pass all filters: the token address is confirmed in the ' +
    'position composition (directly or via LP → vault), the position is a real use-case (DEX LP, lending supply, vault, staking, ' +
    'yield market), liquidity ≥ {liquidity} and source data not older than {hours}h at collection. A position shared by two ' +
    'tokenized assets counts once. This is the main rating.',
  'legend.assets.raw': 'Distinct positions confirmed by contract address before the use-case, liquidity and freshness filters.',
  'legend.assets.protocols': 'Distinct DeFi protocols among the eligible positions.',
  'legend.assets.tokenizedMcap': 'CoinMarketCap market cap of all its tokenized assets. Not DeFi liquidity.',
  'legend.assets.updated': 'When CoinMarketCap and Portals data for this asset was collected.',

  // ---------- asset detail ----------
  'asset.isin': 'ISIN',
  'asset.tokenizedAssets': 'Tokenized Assets',
  'asset.highestApy': 'Highest Reported APY',
  'asset.allPositions': 'All {ticker} positions →',
  'asset.noYield': 'No eligible positions with reported APY',
  'asset.excluded': 'Excluded From Ranking',

  // ---------- tokenized assets page ----------
  'tokenized.title': 'Tokenized Assets',
  'tokenized.intro':
    'Every tokenized stock and ETF in the research basket in one table: issuer, networks, number of eligible DeFi positions ' +
    '(the score), protocols, reported APY and market cap. Filter by underlying, issuer or network and sort by any column.',
  'tokenized.showing': 'Showing {n} of {total} tokenized assets',
  'tokenized.filtered': ' (filtered)',
  'tokenized.empty': 'No tokenized assets match the filters',
  'tokenized.emptyUnderlying': 'CMC tracks no tokenized assets for this underlying',
  'legend.tokenized.underlying':
    'Ticker of the underlying stock or ETF. Open a row for the token details; its rank among tokenized assets of the same underlying ' +
    'is shown on the underlying and issuer pages.',
  'legend.tokenized.token':
    'Symbol of the tokenized asset in CoinMarketCap; the full name is in the tooltip. "Wrapper" marks a wrapper around another token.',
  'legend.tokenized.issuer': 'Issuer of the token, as labeled by CoinMarketCap (not a verified legal entity).',
  'legend.tokenized.networks':
    'Networks where CoinMarketCap lists a contract of the token. Highlighted: indexed by Portals and searched for DeFi positions. ' +
    'Greyed: network not covered by Portals or token not indexed — usage there is unknown, not zero.',
  'legend.tokenized.score':
    'Number of eligible DeFi positions with this token: the token address is confirmed in the position composition (directly or ' +
    'via LP → vault), the position is a real use-case (DEX LP, lending supply, vault, staking, yield market), liquidity ≥ {liquidity} ' +
    'and source data not older than {hours}h at collection. This is the main rating.',
  'legend.tokenized.raw': 'Positions confirmed by contract address before the use-case, liquidity and freshness filters.',
  'legend.tokenized.protocols': 'Distinct DeFi protocols among the eligible positions.',
  'legend.tokenized.directIndirect':
    'Eligible positions holding the token itself (e.g. a DEX pool) / holding it through another position (e.g. a vault that holds an LP with the token).',
  'legend.tokenized.apy':
    'Highest and median reported APY over eligible positions with positive APY. APY above {threshold} is treated as an anomaly and ' +
    'excluded. A snapshot of one position, not an expected return; "—" means no reported yield in the current coverage.',
  'legend.tokenized.mcap': 'CoinMarketCap market cap of this token. "—" means not reported; $0.00 is a zero reported by CoinMarketCap.',

  // ---------- token detail ----------
  'token.caption': 'Tokenized {ticker}',
  'token.issuer': 'Issuer:',
  'token.issuerUnknown': 'unknown',
  'token.cmc': 'CoinMarketCap',
  'token.rank': '#{rank} of {count} in {ticker}',
  'token.excluded': 'Excluded from rankings: {reason}',
  'token.deployments': 'Deployments',
  'token.noContracts': 'CMC lists no contract addresses for this token',
  'token.breakdown.network': 'Network',
  'token.breakdown.category': 'Category',
  'token.breakdown.protocol': 'Protocol',
  'token.positions': 'Positions',
  'token.unconfirmed': 'Unconfirmed Candidates',
  'token.unconfirmedText':
    'Symbol matches on the same network, but the position composition in Portals does not contain this token address (e.g. lending ' +
    'markets that expose only the loan asset). Not counted anywhere.',
  'breakdown.noPositions': 'No confirmed positions',
  'legend.token.deployments':
    'Contracts of the token listed by CoinMarketCap, with the matching Portals network, symbol and decimals. Indexed: Portals knows ' +
    'the contract and its positions were searched. Not indexed / Unsupported by Portals: DeFi usage on that network is unknown, not zero.',
  'legend.token.breakdown':
    'Eligible positions of the token grouped by network, use-case category and DeFi protocol. Raw: before the filters.',
  'legend.token.breakdownTerm': 'Network / Category / Protocol',
  'legend.position.position':
    'A DeFi position indexed by Portals: pool, vault, lending market or yield market. Open a row for details: composition, link path, ' +
    'APY breakdown, volume and flags.',
  'legend.position.protocol': 'DeFi protocol of the position (Portals platform).',
  'legend.position.network': 'Blockchain where the position lives.',
  'legend.position.category':
    'DEX LP: liquidity in a pool with the token. Lending supply: the token supplied to a lending market. Vault: a vault or strategy ' +
    'holding the token or its LP. Yield market: PT/YT and similar markets. Wrapper: a wrapped token, not a use-case.',
  'legend.position.link':
    'Direct: the position holds the token itself. Indirect (n): it holds the token through other positions, n levels deep (e.g. vault ' +
    '→ LP → token); hover for the path. Symbol match: the name matches, but the token address is not in the composition — unconfirmed ' +
    'and never counted.',
  'legend.position.liquidity': 'Pool TVL or vault deposits reported by Portals, in USD. Values of nested positions are never summed.',
  'legend.position.apy':
    'Total APY reported by Portals for this position (base + rewards): a snapshot, not an expected return. ⚠ marks APY above ' +
    '{threshold}, excluded from yield statistics. Borrow APY is a cost and is shown only in the details.',
  'legend.position.status':
    'Eligible: counted in the ratings. Low liquidity: below {liquidity}. Stale: source data older than {hours}h at collection. ' +
    'Not a use-case: wrapper or unknown category. Unconfirmed: symbol match only.',
  'legend.position.updated': 'How long ago Portals last updated the data of this position.',
  'legend.position.token':
    'Tokenized asset used by the position, with its underlying ticker and issuer. A pool with two tracked tokenized assets appears once per token.',

  // ---------- issuers page ----------
  'issuers.title': 'Issuers',
  'issuers.intro':
    'Issuers of tokenized stocks ranked by DeFi readiness: the number of unique eligible DeFi positions that hold their tokenized ' +
    'assets within the research basket, plus a basket-relative score, protocol count, deployments and direct pool liquidity.',
  'issuers.relativeHint':
    'For each asset with at least two issuers: issuer positions / best issuer positions × 100, averaged with equal weight per asset. ' +
    'Provisional: based on search coverage.',
  'legend.issuers.rank': 'Position of the issuer in the current sort.',
  'legend.issuers.issuer':
    'Issuer of tokenized assets, as labeled by CoinMarketCap (not a verified legal entity). Open a row for its tokenized assets and positions.',
  'legend.issuers.score':
    'Unique eligible DeFi positions across all tokenized assets of the issuer in this basket. A position counts as eligible when the token ' +
    'address is confirmed in its composition (directly or via LP → vault), it is a real use-case (DEX LP, lending supply, vault, staking, ' +
    'yield market), liquidity ≥ {liquidity} and source data is not older than {hours}h at collection. A pool shared by two tokenized ' +
    'assets of the same issuer counts once; tokens outside the basket do not count. This is the main rating.',
  'legend.issuers.relative':
    'Basket relative score, 0–100: for every asset with at least two issuers, the issuer’s eligible positions divided by the best ' +
    'issuer’s on that asset × 100, averaged with equal weight per asset. In brackets: number of compared assets. Shows quality per ' +
    'asset regardless of how many assets the issuer covers. Provisional: based on search coverage.',
  'legend.issuers.raw': 'Unique positions confirmed by contract address before the use-case, liquidity and freshness filters.',
  'legend.issuers.protocols': 'Distinct DeFi protocols among the eligible positions.',
  'legend.issuers.assets': 'Underlying assets of the basket covered by the issuer / the issuer’s tokenized assets of them.',
  'legend.issuers.deployments':
    'Token contracts on networks covered by Portals / all contracts listed by CoinMarketCap. Contracts on other networks (Solana, TON, ' +
    'X Layer, …) are not searched: usage there is unknown, not zero.',
  'legend.issuers.directIndirect':
    'Eligible positions holding the tokenized asset itself (e.g. a DEX pool) / holding it through another position (e.g. a vault that holds an LP with the token).',
  'legend.issuers.yield': 'Eligible positions with a positive reported APY (APY above {threshold} excluded as anomaly).',
  'legend.issuers.poolsLiquidity':
    'Sum of liquidity of distinct eligible DEX pools that directly hold the issuer’s tokenized assets. Not the issuer’s TVL or capital: ' +
    'vaults on top of these pools are not added.',

  // ---------- issuer detail ----------
  'issuer.roster': '{n} tokens tracked by CMC in total (not all in this basket)',
  'issuer.tokensInBasket': 'Tokenized Assets In Basket',
  'issuer.allPositions': 'All positions of {name} →',
  'issuer.none': 'No tokenized assets of this issuer in the basket',
  'legend.issuer.asset': 'Ticker of the underlying stock or ETF. Open a row for the token details.',
  'legend.issuer.token': 'Symbol and name of the issuer’s tokenized asset in CoinMarketCap.',
  'legend.issuer.rankInAsset': 'Place of this token among all tokenized versions of the same underlying in the basket, by score.',
  'legend.issuer.score':
    'Number of eligible DeFi positions with this token: the token address is confirmed in the position composition (directly or via ' +
    'LP → vault), the position is a real use-case (DEX LP, lending supply, vault, staking, yield market), liquidity ≥ {liquidity} and ' +
    'source data not older than {hours}h at collection. Ties are broken by protocols, networks and direct positions. This is the main rating.',
  'legend.issuer.liquidity':
    'Liquidity of the token’s best position: the eligible position with the highest reported APY. Not the total liquidity of the token; ' +
    '"—" means no position with reported APY.',
  'legend.issuer.maxApy':
    'Highest reported APY over eligible positions with positive APY. APY above {threshold} is treated as an anomaly and excluded. A ' +
    'snapshot of one position, not an expected return.',
  'legend.issuer.updated': 'Source timestamp of the best position at collection.',

  // ---------- positions page ----------
  'positions.title': 'DeFi Positions',
  'positions.intro':
    'DEX pools, lending markets, vaults and yield markets that hold tokenized stocks, with liquidity, reported APY and the link between ' +
    'the position and the token. Filter by underlying, issuer, network, category or protocol. Reported APY is a snapshot from Portals, ' +
    'not an expected or guaranteed return.',
  'positions.showing': 'Showing {n} token–position links ({d} distinct positions)',
  'positions.empty': 'No confirmed positions in current coverage',
  'positions.symbolMatch': 'Symbol match',
  'positions.direct': 'Direct',
  'positions.indirect': 'Indirect ({depth})',
  'positions.via': 'Via {path}',
  'positions.details': 'Details of {name}',

  // ---------- position dialog ----------
  'dialog.close': 'Close',
  'dialog.status': 'Status',
  'dialog.token': 'Token',
  'dialog.issuerUnknown': 'issuer unknown',
  'dialog.linkToToken': 'Link to token',
  'dialog.direct': '(direct)',
  'dialog.indirect': '(indirect, depth {depth})',
  'dialog.symbolMatchOnly': 'Symbol match only: composition does not contain the token address',
  'dialog.protocol': 'Protocol',
  'dialog.network': 'Network',
  'dialog.category': 'Category',
  'dialog.composition': 'Composition',
  'dialog.positionId': 'Position id',
  'dialog.kind': 'Kind',
  'dialog.kind.poolId': 'Pool id (not an ERC-20 share)',
  'dialog.kind.contract': 'Contract',
  'dialog.kind.notPortable': ', not zappable via Portals',
  'dialog.liquidity': 'Liquidity',
  'dialog.liquidity.pool_tvl': 'pool TVL',
  'dialog.liquidity.vault_tvl': 'deposits (TVL)',
  'dialog.liquidity.token_metric': 'token metric, not market depth',
  'dialog.liquidity.unknown': 'semantics unknown',
  'dialog.reportedApy': 'Reported APY',
  'dialog.baseReward': 'Base / reward APY',
  'dialog.borrow': 'Borrow APY (cost, not yield)',
  'dialog.volume': 'Volume 1d / 7d',
  'dialog.sourceUpdated': 'Source updated',
  'dialog.flags': 'Flags',
  'dialog.source': 'Source: Portals token index. Reported APY is a snapshot of this position, not an expected return.',

  // ---------- summary page ----------
  'summary.title': 'Summary',
  'summary.intro':
    'Market analytics of tokenized stocks in DeFi: which issuers are the most DeFi-ready, which tokenized assets report the highest ' +
    'yield, and how eligible positions split by underlying asset. All figures come from the same data collection as the tables; ' +
    'reported APY is a snapshot, not an expected return.',
  'summary.stat.tokensInDefi': 'Tokenized assets in DeFi',
  'summary.stat.tokensInDefi.hint': '{n} of {total} tokenized assets have at least one eligible position',
  'summary.stat.topIssuer': 'Top issuer',
  'summary.stat.topIssuer.hint': '{share} of eligible positions',
  'summary.stat.topCoverage': 'Top issuer by DeFi coverage',
  'summary.stat.topCoverage.hint': '{n} of {total} tokenized assets ({share}) have at least one eligible position',
  'summary.stat.positionsWithApy': 'Positions with APY > 0',
  'summary.stat.positionsWithApy.hint': 'Distinct eligible positions with a positive reported APY; zero and missing APY excluded',
  'summary.stat.avgApy': 'Average APY',
  'summary.stat.avgApy.hint': 'Mean over distinct eligible positions with APY > 0',
  'summary.stat.maxApy': 'Max APY',
  'summary.stat.tokensWithApy': 'Tokenized assets with APY > 0',
  'summary.stat.tokensWithApy.hint': 'Out of {total} tokenized assets',
  'summary.issuers.title': 'Issuers: DeFi readiness',
  'summary.issuers.share': 'Share of eligible positions',
  'summary.issuers.share.subtitle': 'Which issuer’s tokenized assets are used in DeFi the most (issuer score / sum of scores)',
  'summary.issuers.inDefi': 'Tokenized assets in DeFi',
  'summary.issuers.inDefi.subtitle': 'Share of the issuer’s tokenized assets with at least one eligible position',
  'summary.issuers.inDefi.detail': '{n} of {total} tokenized assets in DeFi',
  'summary.issuers.inDefi.caption': '{n} of {total} assets',
  'summary.issuers.inDefi.tooltip': '{n} of {total} tokenized assets have at least one eligible position',
  'summary.issuers.diversity': 'Protocol diversity',
  'summary.issuers.diversity.subtitle':
    'Distinct DeFi protocols hosting the issuer’s tokenized assets; many pools of one protocol count once',
  'summary.issuers.protocols': '{n, plural, one {protocol} other {protocols}}',
  'summary.issuers.noEligible': 'No eligible positions',
  'summary.eligiblePositions': 'eligible positions',
  'summary.yield.title': 'Most profitable tokenized stocks',
  'summary.yield.text':
    'Only eligible positions with a reported APY above zero are counted: zero or missing APY is left out so it does not drag the ' +
    'average down, APY above {threshold} is excluded as anomaly. {n} of {total} tokenized assets have such positions.',
  'summary.yield.sortBy': 'Sort by',
  'summary.yield.sortAverage': 'Average APY',
  'summary.yield.sortMax': 'Max APY',
  'summary.yield.minPositions': 'Min. positions with APY',
  'summary.yield.show': 'Show',
  'summary.yield.chartAverage': 'Tokenized assets by average APY',
  'summary.yield.chartMax': 'Tokenized assets by max APY',
  'summary.yield.chartSubtitle': 'Showing {n} of {total} tokenized assets · value: average / max APY',
  'summary.yield.empty': 'No tokenized assets with at least {n} positions with APY.',
  'summary.yield.keyAverage': 'Average APY',
  'summary.yield.keyMax': 'Max APY',
  'summary.yield.keyBasket': 'Basket average {value}',
  'summary.yield.positions': '{n} pos.',
  'summary.yield.tooltipStats': 'Avg {average} · Max {max}',
  'summary.yield.tooltipCount': '{n, plural, one {# eligible position with APY > 0} other {# eligible positions with APY > 0}}',
  'summary.underlying.title': 'By underlying asset',
  'summary.underlying.chart': 'Eligible positions by issuer',
  'summary.underlying.chartSubtitle':
    'Each bar is one asset, split by the issuer of the tokenized assets used. Select an asset for its breakdown below.',
  'summary.underlying.picker': 'Underlying asset',
  'summary.underlying.open': 'Open asset page',
  'summary.underlying.inDefi': 'Tokenized assets in DeFi',
  'summary.underlying.inDefi.hint': 'Tokenized assets with at least one eligible position',
  'summary.underlying.readiness': 'DeFi readiness by issuer',
  'summary.underlying.readiness.subtitle': 'Share of {ticker} eligible positions by the issuer of the tokenized asset used',
  'summary.underlying.noEligible': 'No eligible positions found for {ticker} in the current coverage.',
  'summary.underlying.yield': 'Yield by tokenized asset',
  'summary.underlying.yield.subtitle': 'Average and max APY over each token’s positions with APY > 0',
  'summary.underlying.noYield': 'No tokenized {ticker} has an eligible position with APY above zero.',
  'summary.underlying.noApyList': 'No APY > 0: {list}',
  'summary.underlying.select': 'Show the breakdown of {ticker}',
  'legend.summary.eligible':
    'A DeFi position that contains the token, confirmed by its contract address (directly or via LP → vault), is a real use-case ' +
    '(DEX LP, lending supply, vault, staking, yield market), has liquidity ≥ {liquidity} and source data not older than {hours}h at collection.',
  'legend.summary.eligibleTerm': 'Eligible position',
  'legend.summary.share':
    'Issuer score (unique eligible positions of its tokenized assets) divided by the sum of all issuer scores. A pool holding tokenized ' +
    'assets of two issuers counts for each of them.',
  'legend.summary.tokensInDefi':
    'Tokenized assets with at least one eligible position / all tokenized assets of the issuer or underlying in the basket. Shows how ' +
    'much of the issuer’s line-up is usable in DeFi, independent of how many positions each token has.',
  'legend.summary.protocols':
    'Number of distinct DeFi protocols (Uniswap V3, Morpho, Beefy…) with at least one eligible position holding any of the issuer’s ' +
    'tokenized assets. Many pools of the same protocol count once; protocol versions (Uniswap V2 vs V3) count separately.',
  'legend.summary.avgApy':
    'Mean reported APY over distinct eligible positions with APY above zero. Positions with zero or no reported APY are left out, APY ' +
    'above {threshold} is excluded as anomaly. Reported APY of one position is not an expected return.',
  'legend.summary.maxApy': 'Highest reported APY among those positions.',
  'legend.summary.basketAverage':
    'Average APY over all eligible positions with APY > 0 in the basket; drawn as a vertical line on the yield charts.',
  'legend.summary.basketAverageTerm': 'Basket average',
  'legend.summary.pos':
    'Number of eligible positions with APY > 0 behind the token’s average. One position means the average equals the max.',
  'legend.summary.posTerm': 'pos.',

  // ---------- charts (accessibility) ----------
  'chart.donut.summary': '{caption}: {total} in total. {items}',
  'chart.stacked.summary': '{total} in total: {items}',
  'chart.item': '{label} {value} ({share})',

  // ---------- copy address ----------
  'copy.tooltip': 'Copy full address',
  'copy.aria': 'Copy address {address}',
  'copy.success': 'Address copied to clipboard!',
  'copy.fail': 'Failed to copy address',

  // ---------- data labels (categories, statuses, flags) ----------
  'category.dex_lp': 'DEX LP',
  'category.lending_supply': 'Lending supply',
  'category.vault': 'Vault',
  'category.staking': 'Staking',
  'category.yield_market': 'Yield market',
  'category.wrapper': 'Wrapper',
  'category.unknown': 'Unknown',
  'coverage.search.noPositions': 'No positions',
  'coverage.search.noPositions.hint':
    'No confirmed positions in current coverage: Portals search found no position containing this token’s address',
  'coverage.partial': 'Partial fetch',
  'coverage.partial.hint': 'Some deployments were not checked in the last run',
  'coverage.not_indexed': 'Not indexed',
  'coverage.not_indexed.hint': 'Supported network, but Portals does not index this token',
  'coverage.unsupported_by_portals': 'Unsupported by Portals',
  'coverage.unsupported_by_portals.hint': 'All deployments are on networks Portals does not cover: DeFi usage is unknown, not zero',
  'coverage.no_contracts': 'No contracts',
  'coverage.no_contracts.hint': 'CMC lists no contract addresses for this token',
  'coverage.data_unavailable': 'Data unavailable',
  'coverage.data_unavailable.hint': 'Source data could not be fetched',
  'coverage.no_tokens': 'No tokenized assets',
  'coverage.no_tokens.hint': 'CMC tracks no tokenized assets for this underlying',
  'deployment.indexed': 'Indexed',
  'deployment.not_indexed': 'Not indexed',
  'deployment.unsupported_network': 'Unsupported by Portals',
  'deployment.unmapped_network': 'Unmapped network',
  'deployment.not_checked': 'Not checked',
  'deployment.data_unavailable': 'Data unavailable',
  'eligibility.eligible': 'Eligible',
  'eligibility.eligible.hint': 'Confirmed use-case, fresh data, enough liquidity',
  'eligibility.low_liquidity': 'Low liquidity',
  'eligibility.low_liquidity.hint': 'Liquidity below the eligibility threshold',
  'eligibility.stale': 'Stale',
  'eligibility.stale.hint': 'Source data older than the freshness limit at collection time',
  'eligibility.unknown_liquidity': 'Unknown liquidity',
  'eligibility.unknown_liquidity.hint': 'Portals reports no comparable liquidity for this position',
  'eligibility.unknown_freshness': 'Unknown freshness',
  'eligibility.unknown_freshness.hint': 'No source timestamp',
  'eligibility.category_excluded': 'Not a use-case',
  'eligibility.category_excluded.hint': 'Wrapper or unclassified position: kept as raw only',
  'eligibility.unconfirmed': 'Unconfirmed',
  'eligibility.unconfirmed.hint': 'Symbol matches, but the position composition does not contain the token address',
  'underlyingStatus.ok': 'OK',
  'underlyingStatus.not_in_cmc': 'Not tracked by CMC',
  'underlyingStatus.no_tracked_tokens': 'No tokenized assets',
  'underlyingStatus.ambiguous_cmc_match': 'Ambiguous CMC match',
  'underlyingStatus.rwa_id_changed': 'CMC id changed',
  'flag.sentinel_price': 'Pool position, not an ERC-20 share (no share price)',
  'flag.range_details_unknown': 'Concentrated liquidity: range details unknown',
  'flag.negative_volume_net_flow': 'Negative volume reported (net flow)',
  'flag.apy_anomaly': 'APY above review threshold: excluded from yield stats',
  'flag.apy_negative': 'Negative APY reported',
  'flag.apy_missing': 'No APY reported',
  'flag.liquidity_missing': 'No liquidity reported',
  'excluded.derivative': 'Derivative, not a tokenized security',
  'excluded.missing_symbol': 'No symbol in CMC data',
  'excluded.symbol_does_not_match_ticker': 'Symbol does not match the underlying ticker (other share class?)',

  // ---------- page metadata (title, description) ----------
  'meta.basket':
    '{underlyings} stocks and ETFs, {tokens} tokenized assets from {issuers} issuers, {positions} eligible DeFi positions. ' +
    'Data from CoinMarketCap and Portals, collected {date}.',
  'meta.home.title': 'Tokenized Stocks in DeFi — Assets, Issuers & Yield',
  'meta.assets.title': 'Underlying Assets: Tokenized Stocks & ETFs in DeFi',
  'meta.assets.description': 'Stocks and ETFs ranked by DeFi usage of their tokenized assets. {basket}',
  'meta.tokenized.title': 'Tokenized Assets Ranked by DeFi Positions',
  'meta.tokenized.description': 'Tokenized stocks and ETFs ranked by eligible DeFi positions. {basket}',
  'meta.issuers.title': 'Tokenized Stock Issuers Ranked by DeFi Readiness',
  'meta.issuers.description': 'Issuers of tokenized stocks ranked by unique eligible DeFi positions. {basket}',
  'meta.positions.title': 'DeFi Positions with Tokenized Stocks: Pools, Lending, Vaults',
  'meta.positions.description': 'DEX pools, lending markets, vaults and yield markets that hold tokenized stocks. {basket}',
  'meta.summary.title': 'Summary: DeFi Readiness and Yield of Tokenized Stocks',
  'meta.summary.description': 'Which issuer is the most DeFi-ready and which tokenized stocks yield the most (APY > 0 only). {basket}',
  'meta.asset.title': '{name} ({ticker}) Tokenized Stocks — Issuers & DeFi Positions',
  'meta.asset.description':
    '{versions} tokenized assets of {name} ({ticker}) from {issuers} issuers: {positions} eligible DeFi positions in {protocols} ' +
    'protocols, ranked by DeFi usage.',
  'meta.token.title': '{symbol} by {issuer} — Tokenized {ticker} in DeFi',
  'meta.token.description': '{symbol}{name}: tokenized {ticker} by {issuer}. {positions} eligible DeFi positions in {protocols} protocols.',
  'meta.token.nameSuffix': ' ({name})',
  'meta.token.unknownIssuer': 'unknown issuer',
  'meta.issuer.title': '{name}: Tokenized Stocks in DeFi',
  'meta.issuer.description':
    '{name} issues {tokens} tokenized assets of {underlyings} stocks and ETFs in the basket, used in {positions} unique eligible DeFi positions.',
  'meta.notFound.title': 'Page not found',
  'meta.dataset.name': 'Tokenized stocks in DeFi: assets, issuers and positions',
  'meta.dataset.description':
    'Research data set of tokenized stocks and ETFs: tokenized assets tracked by CoinMarketCap, their contract deployments, and the DeFi ' +
    'positions (DEX pools, lending markets, vaults, yield markets) indexed by Portals that hold them, with liquidity and reported APY.',
  'meta.keywords':
    'tokenized stocks, tokenized equities, tokenized ETFs, real-world assets, RWA, tokenized assets in DeFi, DeFi liquidity, tokenized stock issuers, on-chain yield, cp0x'
} as const;

export default en;
