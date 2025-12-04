export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="relative min-h-screen overflow-hidden">
      {/* Animated background */}
      <div className="fixed inset-0 -z-10">
        <div className="absolute inset-0 bg-aurora-gradient" />
        <div className="absolute inset-0 bg-[url('/grid.svg')] bg-center opacity-20" />
        <div className="absolute -top-40 -right-40 h-80 w-80 rounded-full bg-twilight-500/30 blur-[100px] animate-pulse" />
        <div className="absolute -bottom-40 -left-40 h-80 w-80 rounded-full bg-sunset-500/30 blur-[100px] animate-pulse animation-delay-2000" />
      </div>
      
      {children}
    </div>
  );
}

