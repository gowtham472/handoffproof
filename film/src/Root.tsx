import {Composition, staticFile} from 'remotion';
import {Film} from './Film';

export const Root = () => {
	return (
		<Composition
			id="HandoffProofDemo"
			component={Film}
			durationInFrames={4500}
			fps={30}
			width={1920}
			height={1080}
			defaultProps={{music: staticFile('music.mp3')}}
		/>
	);
};
