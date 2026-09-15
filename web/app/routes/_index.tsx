import type { MetaFunction } from "@remix-run/node";
import HomePageStory from '~/components/HomePageStory';
import '~/styles/homepage-story.css';

export const meta: MetaFunction = () => {
  return [
    { title: "Threadify — Service-delivery Intelligence" },
    { name: "description", content: "Threadify captures and validates how your business delivers on every customer request — turning that into intelligence for your teams, systems, and agents." },
  ];
};

export default function Index() {
  return <HomePageStory />;
}
