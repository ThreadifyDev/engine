import type { MetaFunction } from "@remix-run/node";
import HomePageStory from '~/components/HomePageStory';
import '~/styles/homepage-story.css';

export const meta: MetaFunction = () => {
  return [
    { title: "Threadify — Execution Intelligence" },
    { name: "description", content: "A shared referee for work across your services and AI agents. Follow what happens, check your rules, and decide when work can continue." },
  ];
};

export default function Index() {
  return <HomePageStory />;
}
