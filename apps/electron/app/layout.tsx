import "./globals.css";
import localFont from "next/font/local";

const geist = localFont({ src: "./fonts/geist-sans.woff2", variable: "--font-geist", display: "swap", weight: "100 900" });
const geistMono = localFont({ src: "./fonts/geist-mono.woff2", variable: "--font-geist-mono", display: "swap", weight: "100 900" });

export const metadata = {
  title: "Excelsior — Your coding workspace",
  description: "A focused space to think, build, and work with your coding agent.",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={`${geist.variable} ${geistMono.variable}`} suppressHydrationWarning>
      <body className="antialiased">
        {children}
      </body>
    </html>
  );
}
