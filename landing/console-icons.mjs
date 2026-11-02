import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { SquaresFour, Play, FileText, MagnifyingGlass, GearSix, Warning, Check, ArrowRight, SlackLogo, GithubLogo, X, CaretLeft, CaretRight, ChartLineUp, ShieldCheck, List, ArrowUpRight, ArrowLeft, DotsThree, Plus } from '@phosphor-icons/react';

// Use the package's SVG geometry unchanged in the buildless console.
export function consoleIcons() {
  const icons = { dashboard:SquaresFour, play:Play, doc:FileText, search:MagnifyingGlass, filter:GearSix, warn:Warning, check:Check, arrow:ArrowRight, slack:SlackLogo, github:GithubLogo, close:X, 'chev-l':CaretLeft, 'chev-r':CaretRight, chart:ChartLineUp, shield:ShieldCheck, menu:List, external:ArrowUpRight, back:ArrowLeft, more:DotsThree, plus:Plus };
  const symbols = Object.entries(icons).map(([name,Icon]) => {
    const svg = renderToStaticMarkup(createElement(Icon, { weight:'regular' }));
    return '<symbol id="i-'+name+'" viewBox="0 0 256 256">'+svg.replace(/^<svg[^>]*>/,'').replace(/<\/svg>$/,'')+'</symbol>';
  });
  return '<svg xmlns="http://www.w3.org/2000/svg">'+symbols.join('')+'</svg>';
}
